// Package restconf is a RESTCONF (RFC 8040) client library for Go.
package restconf

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
)

const (
	DefaultMaxRetries         int     = 10
	DefaultBackoffMinDelay    int     = 1
	DefaultBackoffMaxDelay    int     = 60
	DefaultBackoffDelayFactor float64 = 1.2
	RestconfDataEndpoint      string  = "/data"
)

type TransientError struct {
	StatusCode   int
	ErrorType    string
	ErrorTag     string
	ErrorAppTag  string
	ErrorPath    string
	ErrorMessage string
	ErrorInfo    string
}

var TransientErrors = [...]TransientError{
	// RESTCONF on IOS-XE intermittently responds with 400 / "inconsistent value"
	{
		StatusCode:   400,
		ErrorTag:     "invalid-value",
		ErrorMessage: "inconsistent value: Device refused one or more commands",
	},
	{
		ErrorTag: "lock-denied",
	},
	{
		ErrorTag: "in-use",
	},
	{
		StatusCode: 500,
	},
	{
		StatusCode: 501,
	},
	{
		StatusCode: 502,
	},
	{
		StatusCode: 503,
	},
	{
		StatusCode: 504,
	},
}

// Client is an HTTP RESTCONF client.
// Use restconf.NewClient to initiate a client.
// This will ensure proper cookie handling and processing of modifiers.
type Client struct {
	// HttpClient is the *http.Client used for API requests.
	HttpClient *http.Client
	// Mutex to synchronize write operations
	mutex sync.Mutex
	// Url is the device url.
	Url string
	// Usr is the device username.
	Usr string
	// Pwd is the device password.
	Pwd string
	// Insecure determines if insecure https connections are allowed.
	Insecure bool
	// Maximum number of retries
	MaxRetries int
	// Minimum delay between two retries
	BackoffMinDelay int
	// Maximum delay between two retries
	BackoffMaxDelay int
	// Backoff delay factor
	BackoffDelayFactor float64
	// True if discovery (RESTCONF API endpoint and capabilities) is complete
	DiscoveryComplete bool
	// Discovered RESTCONF API endpoint
	RestconfEndpoint string
	// RESTCONF capabilities
	Capabilities []string
	// RESTCONF YANG-Patch capability
	YangPatchCapability bool
}

type YangPatchEdit struct {
	Operation string
	Target    string
	Value     Body
}

// NewClient creates a new RESTCONF HTTP client.
// Pass modifiers in to modify the behavior of the client, e.g.
//
//	client, _ := NewClient("https://10.0.0.1", "user", "password", true, RequestTimeout(120))
func NewClient(url, usr, pwd string, insecure bool, mods ...func(*Client)) (*Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	cookieJar, _ := cookiejar.New(nil)
	httpClient := http.Client{
		Timeout:   60 * time.Second,
		Transport: tr,
		Jar:       cookieJar,
	}

	client := Client{
		HttpClient:         &httpClient,
		Url:                url,
		Usr:                usr,
		Pwd:                pwd,
		Insecure:           insecure,
		MaxRetries:         DefaultMaxRetries,
		BackoffMinDelay:    DefaultBackoffMinDelay,
		BackoffMaxDelay:    DefaultBackoffMaxDelay,
		BackoffDelayFactor: DefaultBackoffDelayFactor,
	}

	for _, mod := range mods {
		mod(&client)
	}
	return &client, nil
}

// RequestTimeout modifies the HTTP request timeout from the default of 60 seconds.
func RequestTimeout(x time.Duration) func(*Client) {
	return func(client *Client) {
		client.HttpClient.Timeout = x * time.Second
	}
}

// MaxRetries modifies the maximum number of retries from the default of 2.
func MaxRetries(x int) func(*Client) {
	return func(client *Client) {
		client.MaxRetries = x
	}
}

// BackoffMinDelay modifies the minimum delay between two retries from the default of 4.
func BackoffMinDelay(x int) func(*Client) {
	return func(client *Client) {
		client.BackoffMinDelay = x
	}
}

// BackoffMaxDelay modifies the maximum delay between two retries from the default of 60.
func BackoffMaxDelay(x int) func(*Client) {
	return func(client *Client) {
		client.BackoffMaxDelay = x
	}
}

// BackoffDelayFactor modifies the backoff delay factor from the default of 3.
func BackoffDelayFactor(x float64) func(*Client) {
	return func(client *Client) {
		client.BackoffDelayFactor = x
	}
}

// SkipDiscovery provides the otherwise dynamically discovered capabilities
func SkipDiscovery(restconfEndpoint string, yangPatchCapability bool) func(*Client) {
	return func(client *Client) {
		client.RestconfEndpoint = restconfEndpoint
		client.YangPatchCapability = yangPatchCapability
		client.DiscoveryComplete = true
	}
}

// NewReq creates a new Req request for this client.
func (client *Client) NewReq(method, uri string, body io.Reader, mods ...func(*Req)) Req {
	httpReq, _ := http.NewRequest(method, client.Url+client.RestconfEndpoint+uri, body)
	httpReq.SetBasicAuth(client.Usr, client.Pwd)
	httpReq.Header.Add("Content-Type", "application/yang-data+json")
	httpReq.Header.Add("Accept", "application/yang-data+json")
	req := Req{
		HttpReq: httpReq,
	}
	for _, mod := range mods {
		mod(&req)
	}
	return req
}

// check if response is considered a transient error
func checkTransientError(res Res) bool {
	found := false
	errors := res.Errors.Error
	for _, edit := range res.YangPatchStatus.EditStatus.Edit {
		errors = append(errors, edit.Errors.Error...)
	}
	for _, resError := range errors {
		for _, error := range TransientErrors {
			found = false
			if error.StatusCode != 0 {
				if error.StatusCode == res.StatusCode {
					found = true
				} else {
					continue
				}
			}
			if error.ErrorType != "" {
				if ok, _ := regexp.MatchString(error.ErrorType, resError.ErrorType); ok {
					found = true
				} else {
					continue
				}
			}
			if error.ErrorTag != "" {
				if ok, _ := regexp.MatchString(error.ErrorTag, resError.ErrorTag); ok {
					found = true
				} else {
					continue
				}
			}
			if error.ErrorAppTag != "" {
				if ok, _ := regexp.MatchString(error.ErrorAppTag, resError.ErrorAppTag); ok {
					found = true
				} else {
					continue
				}
			}
			if error.ErrorPath != "" {
				if ok, _ := regexp.MatchString(error.ErrorPath, resError.ErrorPath); ok {
					found = true
				} else {
					continue
				}
			}
			if error.ErrorMessage != "" {
				if ok, _ := regexp.MatchString(error.ErrorMessage, resError.ErrorMessage); ok {
					found = true
				} else {
					continue
				}
			}
			if error.ErrorInfo != "" {
				if ok, _ := regexp.MatchString(error.ErrorInfo, resError.ErrorInfo); ok {
					found = true
				} else {
					continue
				}
			}
			if found {
				break
			}
		}
	}
	return found
}

// Do makes a request.
// Requests for Do are built ouside of the client, e.g.
//
//	req := client.NewReq("GET", "Cisco-IOS-XE-native:native/hostname", nil)
//	res, _ := client.Do(req)
func (client *Client) Do(req Req) (Res, error) {
	// retain the request body across multiple attempts
	var body []byte
	if req.HttpReq.Body != nil {
		body, _ = io.ReadAll(req.HttpReq.Body)
	}

	res := Res{}

	if req.HttpReq.Method != "GET" {
		client.mutex.Lock()
		defer client.mutex.Unlock()
	}

	for attempts := 0; ; attempts++ {
		req.HttpReq.Body = io.NopCloser(bytes.NewBuffer(body))
		log.Printf("[DEBUG] HTTP Request: %s, %s, %s", req.HttpReq.Method, req.HttpReq.URL, req.HttpReq.Body)

		httpRes, err := client.HttpClient.Do(req.HttpReq)
		if err != nil {
			if ok := client.Backoff(attempts); !ok {
				log.Printf("[ERROR] HTTP Connection error occured: %+v", err)
				log.Printf("[DEBUG] Exit from Do method")
				return res, err
			} else {
				log.Printf("[ERROR] HTTP Connection failed: %s, retries: %v", err, attempts)
				continue
			}
		}

		res.StatusCode = httpRes.StatusCode
		defer httpRes.Body.Close()
		bodyBytes, err := io.ReadAll(httpRes.Body)
		if err != nil {
			if ok := client.Backoff(attempts); !ok {
				log.Printf("[ERROR] Cannot decode response body: %+v", err)
				log.Printf("[DEBUG] Exit from Do method")
				return res, err
			} else {
				log.Printf("[ERROR] Cannot decode response body: %s, retries: %v", err, attempts)
				continue
			}
		}

		if httpRes.StatusCode >= 300 && len(bodyBytes) > 0 {
			if req.HttpReq.Header.Get("Content-Type") == "application/yang-data+json" {
				var errors ErrorsRootModel
				err = json.Unmarshal(bodyBytes, &errors)
				if err != nil {
					log.Printf("[DEBUG] Failed to parse RESTCONF errors: %+v", err)
				}
				if len(errors.Errors.Error) > 0 {
					res.Errors = errors.Errors
				} else {
					var errors ErrorsRootNamespaceModel
					err = json.Unmarshal(bodyBytes, &errors)
					if err != nil {
						log.Printf("[DEBUG] Failed to parse RESTCONF errors: %+v", err)
					}
					res.Errors = errors.Errors
				}
				res.YangPatchStatus = YangPatchStatusModel{}
			} else if req.HttpReq.Header.Get("Content-Type") == "application/yang-patch+json" {
				var status YangPatchStatusRootModel
				err = json.Unmarshal(bodyBytes, &status)
				if err != nil {
					log.Printf("[DEBUG] Failed to parse RESTCONF YANG-Patch status response: %+v", err)
				}
				res.YangPatchStatus = status.YangPatchStatus
				res.Errors = status.YangPatchStatus.Errors
			}
		} else {
			res.Errors = ErrorsModel{}
			res.YangPatchStatus = YangPatchStatusModel{}
		}
		res.Res = gjson.ParseBytes(bodyBytes)
		log.Printf("[DEBUG] HTTP Response: %s", res.Res.Raw)

		// exit if object cannot be deleted
		if req.HttpReq.Method == "DELETE" && httpRes.StatusCode == 502 {
			log.Printf("[DEBUG] Exit from Do method")
			break
		}
		// check transient errors
		if checkTransientError(res) {
			log.Printf("[DEBUG] Transient error detected")
			if ok := client.Backoff(attempts); !ok {
				log.Printf("[ERROR] HTTP Request failed: StatusCode %v, RESTCONF errors %+v %+v", httpRes.StatusCode, res.Errors, res.YangPatchStatus)
				log.Printf("[DEBUG] Exit from Do method")
				return res, fmt.Errorf("HTTP Request failed: StatusCode %v, RESTCONF errors %+v %+v", httpRes.StatusCode, res.Errors, res.YangPatchStatus)
			} else {
				log.Printf("[ERROR] HTTP Request failed: StatusCode %v, RESTCONF errors %+v %+v, Retries: %v", httpRes.StatusCode, res.Errors, res.YangPatchStatus, attempts)
				continue
			}
		}
		// do not retry after non-2xx responses
		if httpRes.StatusCode < 200 || httpRes.StatusCode > 299 {
			log.Printf("[ERROR] HTTP Request failed: StatusCode %v, RESTCONF errors %+v %+v", httpRes.StatusCode, res.Errors, res.YangPatchStatus)
			log.Printf("[DEBUG] Exit from Do method")
			return res, fmt.Errorf("HTTP Request failed: StatusCode %v, RESTCONF errors %+v %+v", httpRes.StatusCode, res.Errors, res.YangPatchStatus)
		}
		// check RESTCONF errors
		if len(res.Errors.Error) > 0 {
			if ok := client.Backoff(attempts); !ok {
				log.Printf("[ERROR] RESTCONF Request failed: %+v %+v", res.Errors, res.YangPatchStatus)
				log.Printf("[DEBUG] Exit from Do method")
				return res, fmt.Errorf("RESTCONF Request failed: %+v %+v", res.Errors, res.YangPatchStatus)
			} else {
				log.Printf("[ERROR] RESTCONF Request failed: %+v %+v, Retries: %v", res.Errors, res.YangPatchStatus, attempts)
				continue
			}
		}

		log.Printf("[DEBUG] Exit from Do method")
		break
	}

	if req.Wait && req.HttpReq.Method != "GET" {
		log.Printf("[DEBUG] Waiting for write operation to complete")
		for i := 0; i < 10; i++ {
			wreq := client.NewReq("GET", RestconfDataEndpoint+"/ietf-netconf-monitoring:netconf-state/datastores/datastore", nil)
			wres, err := client.HttpClient.Do(wreq.HttpReq)
			if err != nil {
				return res, err
			}
			defer wres.Body.Close()
			bodyBytes, err := io.ReadAll(wres.Body)
			if err != nil {
				return res, err
			}
			bodyString := string(bodyBytes)
			log.Printf("[DEBUG] HTTP RESTCONF Datastore State Response: %s", bodyString)

			var status DatastoreStatusRootModel
			err = json.Unmarshal(bodyBytes, &status)
			if err != nil {
				log.Printf("[DEBUG] Failed to parse RESTCONF Datastore State Response: %+v", err)
			}

			hasLock := false
			for _, ds := range status.Datastores {
				if ds.Name == "running" && ds.Status == "valid" && len(ds.Locks.PartialLock) > 0 {
					hasLock = true
					break
				}
			}

			if !hasLock {
				log.Printf("[DEBUG] Write operation completed")
				break
			}

			time.Sleep(1 * time.Second)
		}
	}

	return res, nil
}

func (client *Client) Discovery(mods ...func(*Req)) error {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if !client.DiscoveryComplete {
		err := client.discoverRestconfEndpoint()
		if err != nil {
			return err
		}
		client.discoverCapabilities()
		if err != nil {
			return err
		}
		client.DiscoveryComplete = true
	}
	return nil
}

// Discover RESTCONF API endpoint
func (client *Client) discoverRestconfEndpoint(mods ...func(*Req)) error {
	req := client.NewReq("GET", "/.well-known/host-meta", nil, mods...)
	res, err := client.HttpClient.Do(req.HttpReq)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	bodyString := string(bodyBytes)
	log.Printf("[DEBUG] HTTP RESTCONF Discovery Response: %s", bodyString)
	// hack to avoid XML parsing
	re := regexp.MustCompile(`Link rel='restconf' href='(.+)'`)
	matches := re.FindStringSubmatch(bodyString)
	if len(matches) <= 1 {
		return fmt.Errorf("Could not find RESTCONF API endpoint in discovery response: %s", bodyString)
	}
	client.RestconfEndpoint = matches[1]
	log.Printf("[DEBUG] Discovered RESTCONF API endpoint: %s", matches[1])
	return nil
}

// Discover RESTCONF capabilities
func (client *Client) discoverCapabilities(mods ...func(*Req)) error {
	req := client.NewReq("GET", RestconfDataEndpoint+"/ietf-restconf-monitoring:restconf-state/capabilities", nil, mods...)
	res, err := client.HttpClient.Do(req.HttpReq)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	bodyString := string(bodyBytes)
	log.Printf("[DEBUG] HTTP RESTCONF Capabilities Response: %s", bodyString)
	var caps CapabilitiesRootModel
	err = json.Unmarshal(bodyBytes, &caps)
	if err != nil {
		log.Printf("[DEBUG] Failed to parse RESTCONF capabilities: %+v", err)
	}
	client.Capabilities = caps.Capabilities.Capability
	for _, c := range client.Capabilities {
		if c == "urn:ietf:params:restconf:capability:yang-patch:1.0" {
			client.YangPatchCapability = true
		}
	}
	log.Printf("[DEBUG] Discovered RESTCONF capabilities: %v", client.Capabilities)
	return nil
}

// GetData makes a GET request and returns a GJSON result.
func (client *Client) GetData(path string, mods ...func(*Req)) (Res, error) {
	err := client.Discovery()
	if err != nil {
		return Res{}, err
	}
	req := client.NewReq("GET", RestconfDataEndpoint+"/"+path, nil, mods...)
	return client.Do(req)
}

// DeleteData makes a DELETE request and returns a GJSON result.
func (client *Client) DeleteData(path string, mods ...func(*Req)) (Res, error) {
	err := client.Discovery()
	if err != nil {
		return Res{}, err
	}
	req := client.NewReq("DELETE", RestconfDataEndpoint+"/"+path, nil, mods...)
	return client.Do(req)
}

// PostData makes a POST request and returns a GJSON result.
// Hint: Use the Body struct to easily create POST body data.
func (client *Client) PostData(path, data string, mods ...func(*Req)) (Res, error) {
	err := client.Discovery()
	if err != nil {
		return Res{}, err
	}
	req := client.NewReq("POST", RestconfDataEndpoint+"/"+path, strings.NewReader(data), mods...)
	return client.Do(req)
}

// PutData makes a PUT request and returns a GJSON result.
// Hint: Use the Body struct to easily create PUT body data.
func (client *Client) PutData(path, data string, mods ...func(*Req)) (Res, error) {
	err := client.Discovery()
	if err != nil {
		return Res{}, err
	}
	req := client.NewReq("PUT", RestconfDataEndpoint+"/"+path, strings.NewReader(data), mods...)
	return client.Do(req)
}

// PatchData makes a PATCH request and returns a GJSON result.
// Hint: Use the Body struct to easily create PATCH body data.
func (client *Client) PatchData(path, data string, mods ...func(*Req)) (Res, error) {
	err := client.Discovery()
	if err != nil {
		return Res{}, err
	}
	req := client.NewReq("PATCH", RestconfDataEndpoint+"/"+path, strings.NewReader(data), mods...)
	return client.Do(req)
}

// YangPatchData makes a YANG-PATCH (RFC 8072) request and returns a GJSON result.
func (client *Client) YangPatchData(path, patchId, comment string, edits []YangPatchEdit, mods ...func(*Req)) (Res, error) {
	err := client.Discovery()
	if err != nil {
		return Res{}, err
	}
	data := YangPatchRootModel{YangPatch: YangPatchModel{PatchId: patchId, Comment: comment}}
	for i, edit := range edits {
		data.YangPatch.Edit = append(data.YangPatch.Edit, YangPatchEditModel{EditId: strconv.Itoa(i), Operation: edit.Operation, Target: edit.Target, Value: json.RawMessage(edit.Value.Str)})
	}
	json, err := json.Marshal(data)
	if err != nil {
		return Res{}, err
	}
	req := client.NewReq("PATCH", RestconfDataEndpoint+"/"+path, strings.NewReader(string(json)), mods...)
	req.HttpReq.Header.Set("Content-Type", "application/yang-patch+json")
	return client.Do(req)
}

// Create new YangPathEdit for YangPatchData()
func NewYangPatchEdit(operation, target string, value Body) YangPatchEdit {
	return YangPatchEdit{Operation: operation, Target: target, Value: value}
}

// Backoff waits following an exponential backoff algorithm
func (client *Client) Backoff(attempts int) bool {
	log.Printf("[DEBUG] Begining backoff method: attempts %v on %v", attempts, client.MaxRetries)
	if attempts >= client.MaxRetries {
		log.Printf("[DEBUG] Exit from backoff method with return value false")
		return false
	}

	minDelay := time.Duration(client.BackoffMinDelay) * time.Second
	maxDelay := time.Duration(client.BackoffMaxDelay) * time.Second

	min := float64(minDelay)
	backoff := min * math.Pow(client.BackoffDelayFactor, float64(attempts))
	if backoff > float64(maxDelay) {
		backoff = float64(maxDelay)
	}
	backoff = (rand.Float64()/2+0.5)*(backoff-min) + min
	backoffDuration := time.Duration(backoff)
	log.Printf("[TRACE] Start sleeping for %v", backoffDuration.Round(time.Second))
	time.Sleep(backoffDuration)
	log.Printf("[DEBUG] Exit from backoff method with return value true")
	return true
}

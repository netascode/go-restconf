// Package restconf is a RESTCONF (RFC 8040) client library for Go.
package restconf

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

const (
	DefaultMaxRetries         int     = 2
	DefaultBackoffMinDelay    int     = 4
	DefaultBackoffMaxDelay    int     = 60
	DefaultBackoffDelayFactor float64 = 3
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
	TransientError{
		StatusCode:   400,
		ErrorTag:     "invalid-value",
		ErrorMessage: "inconsistent value",
	},
	TransientError{
		ErrorTag: "lock-denied",
	},
	TransientError{
		StatusCode: 500,
	},
	TransientError{
		StatusCode: 501,
	},
	TransientError{
		StatusCode: 502,
	},
	TransientError{
		StatusCode: 503,
	},
	TransientError{
		StatusCode: 504,
	},
}

// Client is an HTTP RESTCONF client.
// Use restconf.NewClient to initiate a client.
// This will ensure proper cookie handling and processing of modifiers.
type Client struct {
	// HttpClient is the *http.Client used for API requests.
	HttpClient *http.Client
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
	// Discovered RESTCONF API endpoint
	RestconfEndpoint string
}

// NewClient creates a new RESTCONF HTTP client.
// Pass modifiers in to modify the behavior of the client, e.g.
//  client, _ := NewClient("https://10.0.0.1", "user", "password", true, RequestTimeout(120))
func NewClient(url, usr, pwd string, insecure bool, mods ...func(*Client)) (Client, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
	}

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
	return client, nil
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

// NewReq creates a new Req request for this client.
func (client Client) NewReq(method, uri string, body io.Reader, mods ...func(*Req)) Req {
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
	for _, resError := range res.Errors.Error {
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
				if ok, _ := regexp.MatchString(error.ErrorType, resError.ErrorTag); ok {
					found = true
				} else {
					continue
				}
			}
			if error.ErrorAppTag != "" {
				if ok, _ := regexp.MatchString(error.ErrorType, resError.ErrorAppTag); ok {
					found = true
				} else {
					continue
				}
			}
			if error.ErrorPath != "" {
				if ok, _ := regexp.MatchString(error.ErrorType, resError.ErrorPath); ok {
					found = true
				} else {
					continue
				}
			}
			if error.ErrorMessage != "" {
				if ok, _ := regexp.MatchString(error.ErrorType, resError.ErrorMessage); ok {
					found = true
				} else {
					continue
				}
			}
			if error.ErrorInfo != "" {
				if ok, _ := regexp.MatchString(error.ErrorType, resError.ErrorInfo); ok {
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
//  req := client.NewReq("GET", "Cisco-IOS-XE-native:native/hostname", nil)
//  res, _ := client.Do(req)
func (client *Client) Do(req Req) (Res, error) {
	// retain the request body across multiple attempts
	var body []byte
	if req.HttpReq.Body != nil {
		body, _ = ioutil.ReadAll(req.HttpReq.Body)
	}

	res := Res{}

	for attempts := 0; ; attempts++ {
		req.HttpReq.Body = ioutil.NopCloser(bytes.NewBuffer(body))
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
		bodyBytes, err := ioutil.ReadAll(httpRes.Body)
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
			var errors ErrorsRoot
			err = json.Unmarshal(bodyBytes, &errors)
			if err != nil {
				log.Printf("[DEBUG] Failed to parse RESTCONF errors: %+v", err)
			}
			res.Errors = errors.Errors
		} else {
			res.Errors.Error = nil
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
			if ok := client.Backoff(attempts); !ok {
				log.Printf("[ERROR] HTTP Request failed: StatusCode %v, RESTCONF errors %+v", httpRes.StatusCode, res.Errors)
				log.Printf("[DEBUG] Exit from Do method")
				return res, fmt.Errorf("HTTP Request failed: StatusCode %v, RESTCONF errors %+v", httpRes.StatusCode, res.Errors)
			} else {
				log.Printf("[ERROR] HTTP Request failed: StatusCode %v, RESTCONF errors %+v, Retries: %v", httpRes.StatusCode, res.Errors, attempts)
				continue
			}
		}
		// do not retry after non-2xx responses
		if httpRes.StatusCode < 200 || httpRes.StatusCode > 299 {
			log.Printf("[ERROR] HTTP Request failed: StatusCode %v, RESTCONF errors %+v", httpRes.StatusCode, res.Errors)
			log.Printf("[DEBUG] Exit from Do method")
			return res, fmt.Errorf("HTTP Request failed: StatusCode %v, RESTCONF errors %+v", httpRes.StatusCode, res.Errors)
		}
		// check RESTCONF errors
		if len(res.Errors.Error) > 0 {
			if ok := client.Backoff(attempts); !ok {
				log.Printf("[ERROR] RESTCONF Request failed: %+v", res.Errors)
				log.Printf("[DEBUG] Exit from Do method")
				return res, fmt.Errorf("RESTCONF Request failed: %+v", res.Errors)
			} else {
				log.Printf("[ERROR] RESTCONF Request failed: %+v, Retries: %v", res.Errors, attempts)
				continue
			}
		}

		log.Printf("[DEBUG] Exit from Do method")
		break
	}

	return res, nil
}

// Discover RESTCONF API endpoint
func (client *Client) discoverRestconfEndpoint(mods ...func(*Req)) error {
	if client.RestconfEndpoint == "" {
		req := client.NewReq("GET", "/.well-known/host-meta", nil, mods...)
		res, err := client.HttpClient.Do(req.HttpReq)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		bodyBytes, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}
		bodyString := string(bodyBytes)
		// hack to avoid XML parsing
		re := regexp.MustCompile(`Link rel='restconf' href='(.+)'`)
		matches := re.FindStringSubmatch(bodyString)
		if len(matches) <= 1 {
			return fmt.Errorf("Could not find RESTCONF API endpoint in discovery response: %s", bodyString)
		}
		client.RestconfEndpoint = matches[1]
	}
	return nil
}

// GetData makes a GET request and returns a GJSON result.
func (client *Client) GetData(path string, mods ...func(*Req)) (Res, error) {
	client.discoverRestconfEndpoint()
	req := client.NewReq("GET", RestconfDataEndpoint+"/"+path, nil, mods...)
	return client.Do(req)
}

// DeleteData makes a DELETE request and returns a GJSON result.
func (client *Client) DeleteData(path string, mods ...func(*Req)) (Res, error) {
	client.discoverRestconfEndpoint()
	req := client.NewReq("DELETE", RestconfDataEndpoint+"/"+path, nil, mods...)
	return client.Do(req)
}

// PostData makes a POST request and returns a GJSON result.
// Hint: Use the Body struct to easily create POST body data.
func (client *Client) PostData(path, data string, mods ...func(*Req)) (Res, error) {
	client.discoverRestconfEndpoint()
	req := client.NewReq("POST", RestconfDataEndpoint+"/"+path, strings.NewReader(data), mods...)
	return client.Do(req)
}

// PutData makes a PUT request and returns a GJSON result.
// Hint: Use the Body struct to easily create PUT body data.
func (client *Client) PutData(path, data string, mods ...func(*Req)) (Res, error) {
	client.discoverRestconfEndpoint()
	req := client.NewReq("PUT", RestconfDataEndpoint+"/"+path, strings.NewReader(data), mods...)
	return client.Do(req)
}

// PatchData makes a PATCH request and returns a GJSON result.
// Hint: Use the Body struct to easily create PATCH body data.
func (client *Client) PatchData(path, data string, mods ...func(*Req)) (Res, error) {
	client.discoverRestconfEndpoint()
	req := client.NewReq("PATCH", RestconfDataEndpoint+"/"+path, strings.NewReader(data), mods...)
	return client.Do(req)
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

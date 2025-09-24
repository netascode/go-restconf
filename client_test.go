package restconf

import (
	"errors"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

const (
	testURL = "https://10.0.0.1"
)

func testClient() *Client {
	client, _ := NewClient(testURL, "usr", "pwd", true, MaxRetries(0))
	gock.InterceptClient(client.HttpClient)
	gock.New(testURL).Get("/.well-known/host-meta").Reply(200).BodyString(`<XRD xmlns='http://docs.oasis-open.org/ns/xri/xrd-1.0'><Link rel='restconf' href='/restconf'/></XRD>`)
	gock.New(testURL).Get("/restconf/data/ietf-restconf-monitoring:restconf-state/capabilities").Reply(200).BodyString(`{"ietf-restconf-monitoring:capabilities": {"capability": ["urn:ietf:params:restconf:capability:yang-patch:1.0"]}}`)
	return client
}

// ErrReader implements the io.Reader interface and fails on Read.
type ErrReader struct{}

// Read mocks failing io.Reader test cases.
func (r ErrReader) Read(buf []byte) (int, error) {
	return 0, errors.New("fail")
}

// TestNewClient tests the NewClient function.
func TestNewClient(t *testing.T) {
	client, _ := NewClient(testURL, "usr", "pwd", true, RequestTimeout(120), MaxRetries(0))
	assert.Equal(t, client.HttpClient.Timeout, 120*time.Second)
	assert.Equal(t, client.MaxRetries, 0)
}

// TestPerRequestTimeout tests per-request timeout functionality.
func TestPerRequestTimeout(t *testing.T) {
	defer gock.Off()
	client := testClient()

	// Test that the timeout modifier function works correctly
	req := client.NewReq("GET", RestconfDataEndpoint+"/url", nil, Timeout(30*time.Second))
	assert.Equal(t, req.Timeout, 30*time.Second)

	// Test that the timeout modifier function works on existing requests
	timeoutFunc := Timeout(45 * time.Second)
	req2 := client.NewReq("GET", RestconfDataEndpoint+"/url", nil)
	timeoutFunc(&req2)
	assert.Equal(t, req2.Timeout, 45*time.Second)

	// Test convenience methods with timeout modifier
	// This mainly tests that the modifier is properly passed through
	gock.New(testURL).Get("/restconf/data/test").Reply(200)
	_, err := client.GetData("test", Timeout(15*time.Second))
	assert.NoError(t, err)
}

// TestDiscoverRestconfEndpoint tests the Client::discoverRestconfEndpoint method.
func TestDiscoverRestconfEndpoint(t *testing.T) {
	defer gock.Off()
	client := testClient()
	client.discoverRestconfEndpoint()
	assert.Equal(t, client.RestconfEndpoint, "/restconf")
}

func TestDiscoverCapabilities(t *testing.T) {
	defer gock.Off()
	client := testClient()
	client.discoverRestconfEndpoint()
	client.discoverCapabilities()
	assert.Equal(t, client.Capabilities, []string{"urn:ietf:params:restconf:capability:yang-patch:1.0"})
	assert.Equal(t, client.YangPatchCapability, true)
}

// TestClientGet tests the Client::GetData method.
func TestClientGetData(t *testing.T) {
	defer gock.Off()
	client := testClient()
	var err error

	// Success
	gock.New(testURL).Get("/restconf/data/url").Reply(200)
	_, err = client.GetData("url")
	assert.NoError(t, err)

	// HTTP error
	gock.New(testURL).Get("/restconf/data/url").ReplyError(errors.New("fail"))
	_, err = client.GetData("url")
	assert.Error(t, err)

	// Invalid HTTP status code
	gock.New(testURL).Get("/restconf/data/url").Reply(405)
	res, _ := client.GetData("url")
	assert.Equal(t, res.StatusCode, 405)

	// Error decoding response body
	gock.New(testURL).
		Get("/restconf/data/url").
		Reply(200).
		Map(func(res *http.Response) *http.Response {
			res.Body = ioutil.NopCloser(ErrReader{})
			return res
		})
	_, err = client.GetData("url")
	assert.Error(t, err)
}

// TestClientPostData tests the Client::PostData method.
func TestClientPostData(t *testing.T) {
	defer gock.Off()
	client := testClient()

	var err error

	// Success
	gock.New(testURL).Post("/restconf/data/url").Reply(200)
	_, err = client.PostData("url", "{}")
	assert.NoError(t, err)

	// HTTP error
	gock.New(testURL).Post("/restconf/data/url").ReplyError(errors.New("fail"))
	_, err = client.PostData("url", "{}")
	assert.Error(t, err)

	// Invalid HTTP status code
	gock.New(testURL).Post("/restconf/data/url").Reply(405)
	res, _ := client.PostData("url", "{}")
	assert.Equal(t, res.StatusCode, 405)

	// Error decoding response body
	gock.New(testURL).
		Post("/restconf/data/url").
		Reply(200).
		Map(func(res *http.Response) *http.Response {
			res.Body = ioutil.NopCloser(ErrReader{})
			return res
		})
	_, err = client.PostData("url", "{}")
	assert.Error(t, err)
}

func TestClientPostDataWait(t *testing.T) {
	defer gock.Off()
	client := testClient()

	gock.New(testURL).Post("/restconf/data/url").Reply(200)
	gock.New(testURL).Get("/restconf/data/ietf-netconf-monitoring:netconf-state/datastores/datastore").Reply(200)
	_, err := client.PostData("url", "{}", Wait)
	assert.NoError(t, err)
}

// TestBackoff tests the Client::Backoff method.
func TestBackoff(t *testing.T) {
	client := testClient()
	client.MaxRetries = 1
	client.BackoffMinDelay = 1
	start := time.Now()
	client.Backoff(0)
	duration := time.Since(start)
	assert.GreaterOrEqual(t, duration.Seconds(), float64(client.BackoffMinDelay))
}

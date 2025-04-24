package restconf

import (
	"encoding/json"
	"net/http"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type YangPatchRootModel struct {
	YangPatch YangPatchModel `json:"ietf-yang-patch:yang-patch"`
}

type YangPatchModel struct {
	PatchId string               `json:"patch-id"`
	Comment string               `json:"comment,omitempty"`
	Edit    []YangPatchEditModel `json:"edit"`
}

type YangPatchEditModel struct {
	EditId    string          `json:"edit-id"`
	Operation string          `json:"operation"`
	Target    string          `json:"target"`
	Point     string          `json:"point,omitempty"`
	Where     string          `json:"where,omitempty"`
	Value     json.RawMessage `json:"value,omitempty"`
}

// Body wraps SJSON for building JSON body strings.
// Usage example:
//
//	Body{}.Set(Cisco-IOS-XE-native:native.hostname", "ROUTER-1").Str
type Body struct {
	Str string
}

// Set sets a JSON path to a value.
func (body Body) Set(path string, value interface{}) Body {
	res, _ := sjson.Set(body.Str, path, value)
	body.Str = res
	return body
}

// SetRaw sets a JSON path to a raw string value.
// This is primarily used for building up nested structures, e.g.:
//
//	Body{}.SetRaw("Cisco-IOS-XE-native:native", Body{}.Set("hostname", "ROUTER-1").Str).Str
func (body Body) SetRaw(path, rawValue string) Body {
	res, _ := sjson.SetRaw(body.Str, path, rawValue)
	body.Str = res
	return body
}

// Res creates a Res object, i.e. a GJSON result object.
func (body Body) Res() Res {
	return Res{Res: gjson.Parse(body.Str)}
}

// Req wraps http.Request for API requests.
type Req struct {
	// HttpReq is the *http.Request object.
	HttpReq *http.Request
	// Wait until write operation is complete.
	Wait bool
}

// Query sets an HTTP query parameter.
//
//	client.GetData("Cisco-IOS-XE-native:native", restconf.Query("content", "config"))
//
// Or set multiple parameters:
//
//	client.GetData("Cisco-IOS-XE-native:native",
//	  restconf.Query("content", "config"),
//	  restconf.Query("depth", "1"))
func Query(k, v string) func(req *Req) {
	return func(req *Req) {
		q := req.HttpReq.URL.Query()
		q.Add(k, v)
		req.HttpReq.URL.RawQuery = q.Encode()
	}
}

// Wait until write operation is complete.
// This is only relevant for POST, PUT or DELETE requests.
func Wait(req *Req) {
	req.Wait = true
}

package restconf

import (
	"github.com/tidwall/gjson"
)

type ErrorsRoot struct {
	Errors Errors `json:"errors"`
}

type Errors struct {
	Error []Error `json:"error"`
}

type Error struct {
	ErrorType    string `json:"error-type"`
	ErrorTag     string `json:"error-tag"`
	ErrorAppTag  string `json:"error-app-tag"`
	ErrorPath    string `json:"error-path"`
	ErrorMessage string `json:"error-message"`
	ErrorInfo    string `json:"error-info"`
}

// Res is an API response returned by client requests.
// Res.Res is a GJSON result, which offers advanced and safe parsing capabilities.
// https://github.com/tidwall/gjson
type Res struct {
	Res        gjson.Result
	StatusCode int
	Errors     Errors
}

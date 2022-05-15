package restconf

import (
	"github.com/tidwall/gjson"
)

type ErrorsRootNamespaceModel struct {
	Errors ErrorsModel `json:"ietf-restconf:errors"`
}

type ErrorsRootModel struct {
	Errors ErrorsModel `json:"errors"`
}

type ErrorsModel struct {
	Error []ErrorModel `json:"error"`
}

type ErrorModel struct {
	ErrorType    string `json:"error-type"`
	ErrorTag     string `json:"error-tag"`
	ErrorAppTag  string `json:"error-app-tag,omitempty"`
	ErrorPath    string `json:"error-path,omitempty"`
	ErrorMessage string `json:"error-message,omitempty"`
	ErrorInfo    string `json:"error-info,omitempty"`
}

type YangPatchStatusRootModel struct {
	YangPatchStatus YangPatchStatusModel `json:"ietf-yang-patch:yang-patch-status"`
}

type YangPatchStatusModel struct {
	PatchId      string                           `json:"patch-id"`
	GlobalStatus YangPatchStatusGlobalStatusModel `json:"global-status"`
	EditStatus   YangPatchStatusEditStatusModel   `json:"edit-status"`
}

type YangPatchStatusGlobalStatusModel struct {
	Ok     bool        `json:"ok"`
	Errors ErrorsModel `json:"errors"`
}

type YangPatchStatusEditStatusModel struct {
	Edit []YangPatchStatusEditStatusEditModel `json:"edit"`
}

type YangPatchStatusEditStatusEditModel struct {
	EditId string      `json:"edit-id"`
	Ok     bool        `json:"ok"`
	Errors ErrorsModel `json:"errors"`
}

type CapabilitiesRootModel struct {
	Capabilities CapabilitiesModel `json:"ietf-restconf-monitoring:capabilities"`
}

type CapabilitiesModel struct {
	Capability []string `json:"capability"`
}

// Res is an API response returned by client requests.
// Res.Res is a GJSON result, which offers advanced and safe parsing capabilities.
// https://github.com/tidwall/gjson
type Res struct {
	Res gjson.Result
	// HTTP response status code
	StatusCode      int
	Errors          ErrorsModel
	YangPatchStatus YangPatchStatusModel
}

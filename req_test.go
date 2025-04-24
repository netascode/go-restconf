package restconf

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSetRaw tests the Body::SetRaw method.
func TestSetRaw(t *testing.T) {
	name := Body{}.SetRaw("a", `{"name":"a"}`).Res().Res.Get("a.name").Str
	assert.Equal(t, "a", name)
}

// TestQuery tests the Query function.
func TestQuery(t *testing.T) {
	httpReq, _ := http.NewRequest("GET", "http://1.1.1.1", nil)
	req := Req{HttpReq: httpReq}
	Query("foo", "bar")(&req)
	Query("comma", "bar,baz")(&req)
	assert.Equal(t, "bar", req.HttpReq.URL.Query().Get("foo"))
	assert.Equal(t, "bar,baz", req.HttpReq.URL.Query().Get("comma"))
}

package restconf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

// TestSetRaw tests the Body::SetRaw method.
func TestSetRaw(t *testing.T) {
	name := Body{}.SetRaw("a", `{"name":"a"}`).Res().Res.Get("a.name").Str
	assert.Equal(t, "a", name)
}

// TestQuery tests the Query function.
func TestQuery(t *testing.T) {
	defer gock.Off()
	client := testClient()

	gock.New(testURL).Get("/url").MatchParam("foo", "bar").Reply(200)
	_, err := client.GetData("/url", Query("foo", "bar"))
	assert.NoError(t, err)

	// Test case for comma-separated parameters
	gock.New(testURL).Get("/url").MatchParam("foo", "bar,baz").Reply(200)
	_, err = client.GetData("/url", Query("foo", "bar,baz"))
	assert.NoError(t, err)
}

package http_test

import (
	"encoding/json"
	nethttp "net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The error contract says every non-2xx body is {"error": ...}. chi's defaults
// answer 404 with plain text and 405 with an empty body.
func TestUnknownRouteReturnsJSONError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{name: "unknown path under v1", method: nethttp.MethodGet, path: "/v1/nope", wantStatus: nethttp.StatusNotFound},
		{name: "unknown path at root", method: nethttp.MethodGet, path: "/nope", wantStatus: nethttp.StatusNotFound},
		{name: "wrong method", method: nethttp.MethodPut, path: "/v1/users", wantStatus: nethttp.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)

			resp := h.do(t, tt.method, tt.path, "", "")

			require.Equal(t, tt.wantStatus, resp.StatusCode)
			assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

			var body map[string]any
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			assert.NotEmpty(t, body["error"])
		})
	}
}

// An oversized body is not malformed JSON: answering 400 sends the caller
// looking for a syntax error in a payload that is perfectly valid.
func TestOversizedBodyReturns413(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	huge := `{"name":"` + strings.Repeat("a", 2<<20) + `"}`
	resp := h.do(t, nethttp.MethodPost, "/v1/users", huge, "")

	require.Equal(t, nethttp.StatusRequestEntityTooLarge, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body["error"], "too large")
}

// A panic must become a JSON 500 and leave the server serving.
func TestHandlerPanicBecomesJSONError(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.users.createPanics = true

	resp := h.do(t, nethttp.MethodPost, "/v1/users", `{"name":"Ada"}`, "")

	require.Equal(t, nethttp.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	raw := bodyString(t, resp)
	assert.Contains(t, raw, "internal server error")
	assert.NotContains(t, raw, "boom", "the panic value must not reach the client")

	// The process is still healthy.
	h.users.createPanics = false
	after := h.do(t, nethttp.MethodGet, "/v1/healthz", "", "")
	assert.Equal(t, nethttp.StatusOK, after.StatusCode)
}

// The depth limit itself is a service rule and is covered there; this only
// pins that the transport passes a large offset through rather than rejecting
// or truncating it on its own.
func TestLargeOffsetReachesTheService(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	resp := h.do(t, nethttp.MethodGet, "/v1/posts?offset=2147483000", "", "key")

	require.Equal(t, nethttp.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(2147483000), h.posts.gotOffset)
}

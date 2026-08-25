package http_test

import (
	nethttp "net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

func TestAuthenticatedRouteRequiresCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header string
	}{
		{name: "no header", header: ""},
		{name: "wrong scheme", header: "Bearer token"},
		{name: "scheme only", header: "ApiKey"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)

			req, err := nethttp.NewRequestWithContext(
				t.Context(), nethttp.MethodGet, h.server.URL+"/v1/users", nil)
			require.NoError(t, err)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}

			resp, err := h.server.Client().Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			// The old middleware answered 403 here; 401 is the correct status
			// for absent or malformed credentials.
			assert.Equal(t, nethttp.StatusUnauthorized, resp.StatusCode)
			assert.NotEmpty(t, resp.Header.Get("WWW-Authenticate"))
			assert.False(t, h.users.authCalled, "must not hit the service without a key")
		})
	}
}

func TestAuthenticatedRouteRejectsUnknownKey(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.users.authErr = domain.ErrUnauthorized

	resp := h.do(t, nethttp.MethodGet, "/v1/users", "", "unknown-key")

	assert.Equal(t, nethttp.StatusUnauthorized, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("WWW-Authenticate"))
	assert.Equal(t, "unknown-key", h.users.gotAPIKey)
}

func TestAuthenticatedRoutePassesUserThrough(t *testing.T) {
	t.Parallel()

	id := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	h := newHarness(t)
	h.users.user = domain.User{ID: id, Name: "Ada", APIKey: "good"}

	resp := h.do(t, nethttp.MethodGet, "/v1/users", "", "good")

	require.Equal(t, nethttp.StatusOK, resp.StatusCode)
	body := decode[map[string]any](t, resp)
	assert.Equal(t, id.String(), body["id"])
	assert.Equal(t, "Ada", body["name"])
}

// Regression test for the most serious defect in the original code: the auth
// middleware called log.Fatal when the user lookup failed, so one database
// error terminated the whole process. A lookup failure must now be an ordinary
// 500 and the server must keep serving.
func TestAuthLookupFailureDoesNotKillTheServer(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.users.authErr = errBoom

	first := h.do(t, nethttp.MethodGet, "/v1/users", "", "key")
	require.Equal(t, nethttp.StatusInternalServerError, first.StatusCode)
	assert.NotContains(t, bodyString(t, first), "boom")

	// The process is still alive and still serving.
	h.users.authErr = nil
	second := h.do(t, nethttp.MethodGet, "/v1/users", "", "key")
	assert.Equal(t, nethttp.StatusOK, second.StatusCode)

	public := h.do(t, nethttp.MethodGet, "/v1/healthz", "", "")
	assert.Equal(t, nethttp.StatusOK, public.StatusCode)
}

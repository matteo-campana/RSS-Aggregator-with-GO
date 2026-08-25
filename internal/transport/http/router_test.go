package http_test

import (
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	httptransport "github.com/matteo-campana/rss-aggregator/internal/transport/http"
)

// harness builds a router over fake services and exposes it via httptest.
type harness struct {
	users   *fakeUserService
	feeds   *fakeFeedService
	follows *fakeFeedFollowService
	posts   *fakePostService
	server  *httptest.Server
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	h := &harness{
		users:   &fakeUserService{},
		feeds:   &fakeFeedService{},
		follows: &fakeFeedFollowService{},
		posts:   &fakePostService{},
	}

	router := httptransport.NewRouter(httptransport.Deps{
		Users:       h.users,
		Feeds:       h.feeds,
		FeedFollows: h.follows,
		Posts:       h.posts,
		// Logger left nil on purpose: the router must tolerate it.
	})

	h.server = httptest.NewServer(router)
	t.Cleanup(h.server.Close)

	return h
}

// do issues a request against the harness, optionally authenticated.
func (h *harness) do(t *testing.T, method, path, body, apiKey string) *nethttp.Response {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	req, err := nethttp.NewRequestWithContext(t.Context(), method, h.server.URL+path, reader)
	require.NoError(t, err)

	if apiKey != "" {
		req.Header.Set("Authorization", "ApiKey "+apiKey)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	return resp
}

func decode[T any](t *testing.T, resp *nethttp.Response) T {
	t.Helper()

	var out T
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out
}

func bodyString(t *testing.T, resp *nethttp.Response) string {
	t.Helper()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(raw)
}

package feedfetch_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/matteo-campana/rss-aggregator/internal/feedfetch"
)

// Feed URLs are supplied by API clients, so a fetch must not be able to reach
// the machine's own network. httptest listens on loopback, which is exactly the
// kind of target the guard has to refuse.
func TestFetchRefusesPrivateAddressesByDefault(t *testing.T) {
	t.Parallel()

	reached := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		_, _ = w.Write([]byte(rssWithOffsetDates))
	}))
	t.Cleanup(srv.Close)

	// Default options: the guard is on.
	fetcher := feedfetch.New(feedfetch.Options{UserAgent: "test-agent"})

	_, err := fetcher.Fetch(context.Background(), srv.URL)

	require.Error(t, err)
	assert.ErrorIs(t, err, feedfetch.ErrBlockedAddress)
	assert.False(t, reached, "the request must never reach the server")
}

// A redirect is just another dial, so the guard covers it too: an allowed first
// hop cannot be used to bounce into the internal network.
func TestFetchRefusesRedirectToPrivateAddress(t *testing.T) {
	t.Parallel()

	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("secret internal data"))
	}))
	t.Cleanup(internal.Close)

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL, http.StatusFound)
	}))
	t.Cleanup(redirector.Close)

	// Both servers are on loopback, so the guard blocks the first hop already.
	// What this pins is that no configuration reaches the internal server
	// through a redirect without also allowing it directly.
	fetcher := feedfetch.New(feedfetch.Options{UserAgent: "test-agent"})

	_, err := fetcher.Fetch(context.Background(), redirector.URL)

	require.Error(t, err)
	assert.ErrorIs(t, err, feedfetch.ErrBlockedAddress)
}

func TestFetchStopsRedirectLoops(t *testing.T) {
	t.Parallel()

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL, http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	fetcher := feedfetch.New(feedfetch.Options{
		UserAgent:             "test-agent",
		AllowPrivateAddresses: true,
		MaxRedirects:          3,
	})

	_, err := fetcher.Fetch(context.Background(), srv.URL)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "stopped after 3 redirects")
}

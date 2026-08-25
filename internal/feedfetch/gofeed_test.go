package feedfetch_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/matteo-campana/rss-aggregator/internal/feedfetch"
)

// serve starts a local server returning the given body. No test here touches
// the network.
func serve(t *testing.T, contentType, body string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return srv
}

const rssWithOffsetDates = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Example Feed</title>
    <link>https://example.com</link>
    <description>An example</description>
    <item>
      <title>With offset date</title>
      <link>https://example.com/1</link>
      <description>First item</description>
      <pubDate>Mon, 24 Aug 2026 10:00:00 +0200</pubDate>
    </item>
    <item>
      <title>Without any date</title>
      <link>https://example.com/2</link>
    </item>
  </channel>
</rss>`

// The previous implementation parsed pubDate with time.RFC1123 only, so an
// RFC1123Z date like "+0200" failed and the item was dropped silently.
func TestFetchParsesRSSWithNumericZoneOffset(t *testing.T) {
	t.Parallel()

	srv := serve(t, "application/rss+xml", rssWithOffsetDates)
	fetcher := feedfetch.New(feedfetch.Options{UserAgent: "test-agent", AllowPrivateAddresses: true})

	feed, err := fetcher.Fetch(context.Background(), srv.URL)
	require.NoError(t, err)

	assert.Equal(t, "Example Feed", feed.Title)
	require.Len(t, feed.Items, 2)

	first := feed.Items[0]
	assert.Equal(t, "With offset date", first.Title)
	assert.Equal(t, "https://example.com/1", first.Link)
	require.NotNil(t, first.PublishedAt, "an RFC1123Z date must parse")
	assert.Equal(t, time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC), *first.PublishedAt,
		"the date must be normalised to UTC")

	// A dateless item is still returned; the scraper decides the fallback.
	assert.Nil(t, feed.Items[1].PublishedAt)
}

const atomFeed = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Example</title>
  <subtitle>An atom feed</subtitle>
  <link href="https://example.com/"/>
  <updated>2026-08-24T10:00:00Z</updated>
  <entry>
    <title>Atom entry</title>
    <link href="https://example.com/entry"/>
    <id>urn:uuid:00000000-0000-0000-0000-000000000001</id>
    <published>2026-08-24T09:00:00Z</published>
    <updated>2026-08-24T10:00:00Z</updated>
    <summary>Summary text</summary>
  </entry>
</feed>`

// The hand-rolled encoding/xml parser understood RSS only; Atom feeds produced
// an empty channel and no items at all.
func TestFetchParsesAtom(t *testing.T) {
	t.Parallel()

	srv := serve(t, "application/atom+xml", atomFeed)
	fetcher := feedfetch.New(feedfetch.Options{UserAgent: "test-agent", AllowPrivateAddresses: true})

	feed, err := fetcher.Fetch(context.Background(), srv.URL)
	require.NoError(t, err)

	assert.Equal(t, "Atom Example", feed.Title)
	require.Len(t, feed.Items, 1)
	assert.Equal(t, "Atom entry", feed.Items[0].Title)
	assert.Equal(t, "https://example.com/entry", feed.Items[0].Link)
	require.NotNil(t, feed.Items[0].PublishedAt)
	assert.Equal(t, time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC), *feed.Items[0].PublishedAt)
}

const atomUpdatedOnly = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Updated only</title>
  <updated>2026-08-24T10:00:00Z</updated>
  <entry>
    <title>Entry</title>
    <link href="https://example.com/entry"/>
    <id>urn:uuid:2</id>
    <updated>2026-08-24T11:30:00Z</updated>
  </entry>
</feed>`

func TestFetchFallsBackToUpdatedDate(t *testing.T) {
	t.Parallel()

	srv := serve(t, "application/atom+xml", atomUpdatedOnly)
	fetcher := feedfetch.New(feedfetch.Options{UserAgent: "test-agent", AllowPrivateAddresses: true})

	feed, err := fetcher.Fetch(context.Background(), srv.URL)
	require.NoError(t, err)

	require.Len(t, feed.Items, 1)
	require.NotNil(t, feed.Items[0].PublishedAt)
	assert.Equal(t, time.Date(2026, 8, 24, 11, 30, 0, 0, time.UTC), *feed.Items[0].PublishedAt)
}

func TestFetchRejectsMalformedFeed(t *testing.T) {
	t.Parallel()

	srv := serve(t, "application/rss+xml", "<rss><channel><title>oops")
	fetcher := feedfetch.New(feedfetch.Options{UserAgent: "test-agent", AllowPrivateAddresses: true})

	_, err := fetcher.Fetch(context.Background(), srv.URL)

	assert.Error(t, err)
}

func TestFetchReportsHTTPErrors(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	fetcher := feedfetch.New(feedfetch.Options{UserAgent: "test-agent", AllowPrivateAddresses: true})

	_, err := fetcher.Fetch(context.Background(), srv.URL)

	assert.Error(t, err)
}

func TestFetchHonoursContextCancellation(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		_, _ = w.Write([]byte(rssWithOffsetDates))
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	fetcher := feedfetch.New(feedfetch.Options{UserAgent: "test-agent", AllowPrivateAddresses: true})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := fetcher.Fetch(ctx, srv.URL)

	assert.Error(t, err, "a cancelled context must abort the fetch")
}

func TestFetchSendsUserAgent(t *testing.T) {
	t.Parallel()

	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(rssWithOffsetDates))
	}))
	t.Cleanup(srv.Close)

	fetcher := feedfetch.New(feedfetch.Options{UserAgent: "rss-aggregator/test", AllowPrivateAddresses: true})

	_, err := fetcher.Fetch(context.Background(), srv.URL)
	require.NoError(t, err)

	assert.Equal(t, "rss-aggregator/test", <-got)
}

// The scraper fans out goroutines over one Fetcher, so Fetch must be safe for
// concurrent use. Run with -race, this pins that guarantee.
func TestFetchIsSafeForConcurrentUse(t *testing.T) {
	t.Parallel()

	srv := serve(t, "application/rss+xml", rssWithOffsetDates)
	fetcher := feedfetch.New(feedfetch.Options{UserAgent: "test-agent", AllowPrivateAddresses: true})

	done := make(chan error, 8)
	for range 8 {
		go func() {
			_, err := fetcher.Fetch(context.Background(), srv.URL)
			done <- err
		}()
	}

	for range 8 {
		assert.NoError(t, <-done)
	}
}

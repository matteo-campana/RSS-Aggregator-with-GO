// Package feedfetch retrieves and parses remote feeds.
//
// It is the only package that knows about gofeed; everything upstream works
// with domain.FetchedFeed, so replacing the parser never reaches past here.
//
// It also owns the HTTP client used to reach feeds. That client is built here
// rather than injected so every fetch necessarily goes through the dial guard
// in safedial.go: a feed URL comes from an API client, so the connection it
// causes must never be able to reach the machine's own network.
package feedfetch

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// DefaultMaxBytes is the default per-feed response cap.
const DefaultMaxBytes int64 = 10 << 20 // 10 MiB

const (
	defaultTimeout      = 30 * time.Second
	defaultMaxRedirects = 5
)

// Options configures a Fetcher.
type Options struct {
	UserAgent string
	// Timeout bounds a single fetch, connection setup included.
	Timeout time.Duration
	// MaxBytes caps the response size so one hostile feed cannot exhaust memory.
	MaxBytes int64
	// MaxRedirects caps a redirect chain.
	MaxRedirects int
	// AllowPrivateAddresses disables the guard that refuses to connect to
	// loopback, private, link-local and other non-public addresses.
	//
	// Leave it false in production: feed URLs are supplied by API clients, and
	// without the guard one can point the scraper at a cloud metadata endpoint
	// or an internal service. Tests that serve feeds from localhost set it.
	AllowPrivateAddresses bool
}

// Fetcher retrieves feeds over HTTP and parses RSS, Atom and JSON Feed.
type Fetcher struct {
	client    *http.Client
	userAgent string
	maxBytes  int64
}

// New builds a Fetcher and the HTTP client it uses.
func New(opts Options) *Fetcher {
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = DefaultMaxBytes
	}
	if opts.MaxRedirects <= 0 {
		opts.MaxRedirects = defaultMaxRedirects
	}

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	if !opts.AllowPrivateAddresses {
		dialer.Control = guardAddress
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialer.DialContext
	// Feeds are many and mostly unrelated hosts; keep a lid on per-host fan-out
	// so one popular host cannot absorb the whole connection budget.
	transport.MaxConnsPerHost = 8
	transport.MaxIdleConnsPerHost = 2

	client := &http.Client{
		Timeout:   opts.Timeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= opts.MaxRedirects {
				return fmt.Errorf("stopped after %d redirects", opts.MaxRedirects)
			}
			return nil
		},
	}

	return &Fetcher{
		client:    client,
		userAgent: opts.UserAgent,
		maxBytes:  opts.MaxBytes,
	}
}

// Fetch retrieves and parses the feed at url.
func (f *Fetcher) Fetch(ctx context.Context, url string) (domain.FetchedFeed, error) {
	// A gofeed.Parser lazily initialises its translators on first use, which is
	// a data race when one parser is shared across the scraper's goroutines.
	// Building a parser per call avoids that entirely; the expensive resource,
	// the HTTP client, is still shared.
	parser := gofeed.NewParser()
	parser.Client = f.client
	parser.UserAgent = f.userAgent
	parser.MaxByteSize = f.maxBytes

	// Note the argument order: gofeed takes the URL first and the context second.
	parsed, err := parser.ParseURLWithContext(url, ctx)
	if err != nil {
		return domain.FetchedFeed{}, fmt.Errorf("fetch feed %q: %w", url, err)
	}
	if parsed == nil {
		return domain.FetchedFeed{}, fmt.Errorf("fetch feed %q: empty response", url)
	}

	return toDomain(parsed), nil
}

func toDomain(f *gofeed.Feed) domain.FetchedFeed {
	items := make([]domain.FetchedItem, 0, len(f.Items))
	for _, item := range f.Items {
		if item == nil {
			continue
		}
		items = append(items, domain.FetchedItem{
			Title:       item.Title,
			Description: item.Description,
			Link:        item.Link,
			PublishedAt: publishedAt(item),
		})
	}

	return domain.FetchedFeed{
		Title:       f.Title,
		Description: f.Description,
		Link:        f.Link,
		Items:       items,
	}
}

// publishedAt prefers the publication date and falls back to the update date.
//
// gofeed already understands the many date formats found in the wild, which is
// what the previous single time.RFC1123 parse could not do: any other format
// made every item of that feed be dropped silently.
func publishedAt(item *gofeed.Item) *time.Time {
	for _, candidate := range []*time.Time{item.PublishedParsed, item.UpdatedParsed} {
		if candidate != nil {
			utc := candidate.UTC()
			return &utc
		}
	}
	return nil
}

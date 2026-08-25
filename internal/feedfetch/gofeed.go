// Package feedfetch retrieves and parses remote feeds.
//
// It is the only package that knows about gofeed; everything upstream works
// with domain.FetchedFeed, so replacing the parser never reaches past here.
package feedfetch

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// Fetcher retrieves feeds over HTTP and parses RSS, Atom and JSON Feed.
type Fetcher struct {
	client    *http.Client
	userAgent string
	// maxBytes caps the response size so one hostile feed cannot exhaust memory.
	maxBytes int64
}

// Option customises a Fetcher.
type Option func(*Fetcher)

// WithMaxBytes caps the number of bytes read from a single feed.
func WithMaxBytes(n int64) Option {
	return func(f *Fetcher) { f.maxBytes = n }
}

// DefaultMaxBytes is the default per-feed response cap.
const DefaultMaxBytes int64 = 10 << 20 // 10 MiB

// New builds a Fetcher over a shared HTTP client.
//
// The client is shared so connections are reused across feeds; the previous
// implementation built a fresh http.Client for every single fetch.
func New(client *http.Client, userAgent string, opts ...Option) *Fetcher {
	if client == nil {
		client = http.DefaultClient
	}
	f := &Fetcher{client: client, userAgent: userAgent, maxBytes: DefaultMaxBytes}
	for _, opt := range opts {
		opt(f)
	}
	return f
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

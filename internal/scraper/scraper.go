// Package scraper polls the registered feeds on a timer and stores new posts.
//
// Like the service package it declares the narrow interfaces it needs rather
// than depending on a concrete repository, so the whole loop is exercised in
// tests with in-memory fakes and no network.
package scraper

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// FeedRepository supplies the scheduling half of feed storage. It deliberately
// cannot create or list feeds: that is the service layer's concern.
type FeedRepository interface {
	// NextToFetch returns feeds never fetched, or last fetched before
	// notFetchedSince, oldest first.
	NextToFetch(ctx context.Context, limit int32, notFetchedSince time.Time) ([]domain.Feed, error)
	MarkFetched(ctx context.Context, id uuid.UUID, at time.Time) error
}

// PostWriter stores scraped posts. A duplicate URL must surface as
// domain.ErrConflict.
type PostWriter interface {
	Create(ctx context.Context, p domain.Post) error
}

// FeedFetcher retrieves and parses a remote feed.
type FeedFetcher interface {
	Fetch(ctx context.Context, url string) (domain.FetchedFeed, error)
}

// Clock supplies the current time.
type Clock interface {
	Now() time.Time
}

// IDGenerator supplies new post identifiers.
type IDGenerator interface {
	NewID() uuid.UUID
}

// Options tunes the scraping loop.
type Options struct {
	// Concurrency bounds both the batch size and the number of feeds fetched
	// in parallel. It is int32 because that is what the repository's batch
	// limit takes; widening to int for the semaphore is always safe.
	Concurrency int32
	// Interval is the delay between two passes.
	Interval time.Duration
	// RequestTimeout bounds the work done on a single feed.
	RequestTimeout time.Duration
}

// maxConcurrency caps how many feeds may be refreshed in one pass.
const maxConcurrency int32 = 10_000

// Scraper periodically refreshes the least recently fetched feeds.
type Scraper struct {
	feeds   FeedRepository
	posts   PostWriter
	fetcher FeedFetcher
	clock   Clock
	ids     IDGenerator
	log     *slog.Logger
	opts    Options
}

// New wires a Scraper with its dependencies.
func New(
	feeds FeedRepository,
	posts PostWriter,
	fetcher FeedFetcher,
	clock Clock,
	ids IDGenerator,
	log *slog.Logger,
	opts Options,
) *Scraper {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 1
	}
	if opts.Concurrency > maxConcurrency {
		opts.Concurrency = maxConcurrency
	}
	if opts.Interval <= 0 {
		opts.Interval = time.Minute
	}
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = 30 * time.Second
	}
	return &Scraper{
		feeds:   feeds,
		posts:   posts,
		fetcher: fetcher,
		clock:   clock,
		ids:     ids,
		log:     log,
		opts:    opts,
	}
}

// Run scrapes immediately and then once per interval, returning when ctx is
// cancelled.
//
// The previous implementation looped forever on context.Background() with no
// way to stop it; Run now participates in graceful shutdown.
func (s *Scraper) Run(ctx context.Context) error {
	s.log.Info("scraper started",
		"concurrency", s.opts.Concurrency,
		"interval", s.opts.Interval,
	)

	ticker := time.NewTicker(s.opts.Interval)
	defer ticker.Stop()

	for {
		if ctx.Err() != nil {
			s.log.Info("scraper stopped")
			return nil
		}

		s.runOnce(ctx)

		select {
		case <-ctx.Done():
			s.log.Info("scraper stopped")
			return nil
		case <-ticker.C:
		}
	}
}

// runOnce performs a single scraping pass over one batch of feeds.
func (s *Scraper) runOnce(ctx context.Context) {
	// A feed is due once a full interval has passed since it was last fetched.
	notFetchedSince := s.clock.Now().Add(-s.opts.Interval)

	feeds, err := s.feeds.NextToFetch(ctx, s.opts.Concurrency, notFetchedSince)
	if err != nil {
		if ctx.Err() == nil {
			s.log.Error("select feeds to fetch", "error", err)
		}
		return
	}
	if len(feeds) == 0 {
		return
	}

	semaphore := make(chan struct{}, int(s.opts.Concurrency))
	var wg sync.WaitGroup

	for _, feed := range feeds {
		wg.Add(1)
		go func(feed domain.Feed) {
			defer wg.Done()

			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}

			s.scrapeFeed(ctx, feed)
		}(feed)
	}

	wg.Wait()
}

// scrapeFeed refreshes a single feed.
func (s *Scraper) scrapeFeed(ctx context.Context, feed domain.Feed) {
	ctx, cancel := context.WithTimeout(ctx, s.opts.RequestTimeout)
	defer cancel()

	// Mark first so a feed that always fails to parse does not starve the rest
	// of the rotation.
	if err := s.feeds.MarkFetched(ctx, feed.ID, s.clock.Now()); err != nil {
		if ctx.Err() == nil {
			s.log.Error("mark feed as fetched", "feed_id", feed.ID, "error", err)
		}
		return
	}

	fetched, err := s.fetcher.Fetch(ctx, feed.URL)
	if err != nil {
		if ctx.Err() == nil {
			s.log.Warn("fetch feed", "feed", feed.Name, "url", feed.URL, "error", err)
		}
		return
	}

	var created, duplicates, skipped, failed, processed int
	var stopReason string

	for _, item := range fetched.Items {
		err := s.savePost(ctx, feed, item)
		processed++

		switch {
		case err == nil:
			created++
		case errors.Is(err, errSkipItem):
			skipped++
		case errors.Is(err, domain.ErrConflict):
			// Already stored on an earlier pass: the expected steady state.
			duplicates++
		case errors.Is(err, domain.ErrNotFound):
			// The feed was deleted while we were fetching it, so every
			// remaining item would fail its foreign key the same way. Report it
			// once instead of once per item.
			stopReason = "feed removed while it was being scraped"
		case ctx.Err() != nil:
			// The per-feed budget ran out. The feed was already marked as
			// fetched, so without this the remaining items would be dropped
			// with no trace at all.
			stopReason = "per-feed timeout reached before all items were stored"
		default:
			failed++
			s.log.Error("store post", "feed", feed.Name, "url", item.Link, "error", err)
		}

		if stopReason != "" {
			break
		}
	}

	attrs := []any{
		"feed", feed.Name,
		"url", feed.URL,
		"items", len(fetched.Items),
		"created", created,
		"duplicates", duplicates,
		"skipped", skipped,
		"failed", failed,
	}

	if stopReason != "" {
		s.log.Warn("feed scrape stopped early",
			append(attrs, "reason", stopReason, "unprocessed", len(fetched.Items)-processed)...)
		return
	}

	s.log.Info("feed scraped", attrs...)
}

// errSkipItem marks an item that cannot be stored and should not be reported
// as a failure.
var errSkipItem = errors.New("item skipped")

func (s *Scraper) savePost(ctx context.Context, feed domain.Feed, item domain.FetchedItem) error {
	link := strings.TrimSpace(item.Link)
	if link == "" {
		// posts.url is NOT NULL UNIQUE, so link-less items would all collide
		// with each other on the empty string.
		return errSkipItem
	}

	now := s.clock.Now()

	// A missing date used to drop the item silently. Falling back to the time
	// we observed it keeps the post rather than losing it.
	publishedAt := now
	if item.PublishedAt != nil {
		publishedAt = item.PublishedAt.UTC()
	}

	return s.posts.Create(ctx, domain.Post{
		ID:          s.ids.NewID(),
		CreatedAt:   now,
		UpdatedAt:   now,
		Title:       optionalText(item.Title),
		Description: optionalText(item.Description),
		PublishedAt: publishedAt,
		URL:         link,
		FeedID:      feed.ID,
	})
}

// optionalText maps an empty string to a NULL column value.
func optionalText(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

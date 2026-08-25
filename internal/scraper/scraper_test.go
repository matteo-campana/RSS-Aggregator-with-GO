package scraper_test

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
	"github.com/matteo-campana/rss-aggregator/internal/scraper"
)

var errBoom = errors.New("boom")

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type seqIDs struct {
	mu sync.Mutex
	n  int
}

func (g *seqIDs) NewID() uuid.UUID {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	return uuid.New()
}

type fakeFeedRepo struct {
	mu sync.Mutex

	feeds              []domain.Feed
	nextErr            error
	markErr            error
	marked             []uuid.UUID
	markedAt           []time.Time
	nextCalls          int
	gotBatchCap        int32
	gotNotFetchedSince time.Time
}

func (r *fakeFeedRepo) NextToFetch(
	_ context.Context,
	limit int32,
	notFetchedSince time.Time,
) ([]domain.Feed, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextCalls++
	r.gotBatchCap = limit
	r.gotNotFetchedSince = notFetchedSince
	if r.nextErr != nil {
		return nil, r.nextErr
	}
	return r.feeds, nil
}

func (r *fakeFeedRepo) MarkFetched(_ context.Context, id uuid.UUID, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.markErr != nil {
		return r.markErr
	}
	r.marked = append(r.marked, id)
	r.markedAt = append(r.markedAt, at)
	return nil
}

func (r *fakeFeedRepo) markedIDs() []uuid.UUID {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]uuid.UUID(nil), r.marked...)
}

type fakePostWriter struct {
	mu sync.Mutex

	created   []domain.Post
	err       error
	attempted int
}

func (w *fakePostWriter) Create(_ context.Context, p domain.Post) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.attempted++
	if w.err != nil {
		return w.err
	}
	w.created = append(w.created, p)
	return nil
}

func (w *fakePostWriter) attempts() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.attempted
}

func (w *fakePostWriter) posts() []domain.Post {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]domain.Post(nil), w.created...)
}

type fakeFetcher struct {
	mu sync.Mutex

	feed domain.FetchedFeed
	err  error
	urls []string
}

func (f *fakeFetcher) Fetch(_ context.Context, url string) (domain.FetchedFeed, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.urls = append(f.urls, url)
	if f.err != nil {
		return domain.FetchedFeed{}, f.err
	}
	return f.feed, nil
}

func testLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func newScraper(
	t *testing.T,
	feeds *fakeFeedRepo,
	posts *fakePostWriter,
	fetcher *fakeFetcher,
	now time.Time,
) *scraper.Scraper {
	t.Helper()

	return scraper.New(feeds, posts, fetcher, fixedClock{now: now}, &seqIDs{}, testLogger(), scraper.Options{
		Concurrency:    2,
		Interval:       time.Hour, // long enough that only the first pass runs
		RequestTimeout: time.Second,
	})
}

// runOnePass runs the scraper until it has completed its immediate first pass,
// then cancels it.
func runOnePass(t *testing.T, s *scraper.Scraper) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	// The interval is an hour, so once the first pass has done its work the
	// loop is parked on the ticker; cancelling then unblocks Run.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err, "cancellation is a clean stop, not an error")
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

func TestScraperStoresItems(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	published := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	feedID := uuid.New()

	title, description := "Item title", "Item description"
	feeds := &fakeFeedRepo{feeds: []domain.Feed{{ID: feedID, Name: "Example", URL: "https://example.com/feed"}}}
	posts := &fakePostWriter{}
	fetcher := &fakeFetcher{feed: domain.FetchedFeed{
		Items: []domain.FetchedItem{{
			Title:       title,
			Description: description,
			Link:        "https://example.com/1",
			PublishedAt: &published,
		}},
	}}

	runOnePass(t, newScraper(t, feeds, posts, fetcher, now))

	stored := posts.posts()
	require.Len(t, stored, 1)
	assert.Equal(t, "https://example.com/1", stored[0].URL)
	assert.Equal(t, feedID, stored[0].FeedID)
	assert.Equal(t, published, stored[0].PublishedAt)
	require.NotNil(t, stored[0].Title)
	assert.Equal(t, title, *stored[0].Title)
	assert.Equal(t, now, stored[0].CreatedAt)

	assert.Equal(t, []uuid.UUID{feedID}, feeds.markedIDs())
	assert.Equal(t, int32(2), feeds.gotBatchCap, "the batch is bounded by the concurrency")

	// Only feeds a full interval stale are due; without this every replica
	// refetches the same feeds on every tick.
	assert.Equal(t, now.Add(-time.Hour), feeds.gotNotFetchedSince)
}

// The feed can be deleted while it is being fetched. Every remaining item then
// fails its foreign key, and reporting that once beats one error line per item.
func TestScraperStopsWhenFeedIsRemovedMidScrape(t *testing.T) {
	t.Parallel()

	feeds := &fakeFeedRepo{feeds: []domain.Feed{{ID: uuid.New(), URL: "https://example.com/feed"}}}
	posts := &fakePostWriter{err: domain.ErrNotFound}

	items := make([]domain.FetchedItem, 50)
	for i := range items {
		items[i] = domain.FetchedItem{Link: "https://example.com/" + strconv.Itoa(i)}
	}
	fetcher := &fakeFetcher{feed: domain.FetchedFeed{Items: items}}

	runOnePass(t, newScraper(t, feeds, posts, fetcher, time.Now().UTC()))

	assert.Equal(t, 1, posts.attempts(),
		"the loop must stop at the first foreign-key failure, not retry every item")
}

// A duplicate URL is the expected steady state once a feed has been seen; it
// must not be logged or treated as a failure. The original code detected this
// by matching the Italian text "chiave duplicato".
func TestScraperTreatsDuplicatesAsExpected(t *testing.T) {
	t.Parallel()

	feeds := &fakeFeedRepo{feeds: []domain.Feed{{ID: uuid.New(), URL: "https://example.com/feed"}}}
	posts := &fakePostWriter{err: domain.ErrConflict}
	fetcher := &fakeFetcher{feed: domain.FetchedFeed{
		Items: []domain.FetchedItem{{Link: "https://example.com/1"}, {Link: "https://example.com/2"}},
	}}

	// Completing without panicking or stalling is the assertion.
	runOnePass(t, newScraper(t, feeds, posts, fetcher, time.Now().UTC()))

	assert.Empty(t, posts.posts())
}

// A missing date used to make the scraper skip the item entirely.
func TestScraperFallsBackWhenItemHasNoDate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	feeds := &fakeFeedRepo{feeds: []domain.Feed{{ID: uuid.New(), URL: "https://example.com/feed"}}}
	posts := &fakePostWriter{}
	fetcher := &fakeFetcher{feed: domain.FetchedFeed{
		Items: []domain.FetchedItem{{Link: "https://example.com/1", PublishedAt: nil}},
	}}

	runOnePass(t, newScraper(t, feeds, posts, fetcher, now))

	stored := posts.posts()
	require.Len(t, stored, 1, "a dateless item must still be stored")
	assert.Equal(t, now, stored[0].PublishedAt)
}

// posts.url is NOT NULL UNIQUE, so link-less items would all collide on "".
func TestScraperSkipsItemsWithoutLink(t *testing.T) {
	t.Parallel()

	feeds := &fakeFeedRepo{feeds: []domain.Feed{{ID: uuid.New(), URL: "https://example.com/feed"}}}
	posts := &fakePostWriter{}
	fetcher := &fakeFetcher{feed: domain.FetchedFeed{
		Items: []domain.FetchedItem{
			{Title: "no link", Link: "   "},
			{Title: "ok", Link: "https://example.com/1"},
		},
	}}

	runOnePass(t, newScraper(t, feeds, posts, fetcher, time.Now().UTC()))

	stored := posts.posts()
	require.Len(t, stored, 1)
	assert.Equal(t, "https://example.com/1", stored[0].URL)
}

func TestScraperMapsEmptyTextToNull(t *testing.T) {
	t.Parallel()

	feeds := &fakeFeedRepo{feeds: []domain.Feed{{ID: uuid.New(), URL: "https://example.com/feed"}}}
	posts := &fakePostWriter{}
	fetcher := &fakeFetcher{feed: domain.FetchedFeed{
		Items: []domain.FetchedItem{{Link: "https://example.com/1", Title: "  ", Description: ""}},
	}}

	runOnePass(t, newScraper(t, feeds, posts, fetcher, time.Now().UTC()))

	stored := posts.posts()
	require.Len(t, stored, 1)
	assert.Nil(t, stored[0].Title)
	assert.Nil(t, stored[0].Description)
}

func TestScraperSurvivesFetchFailure(t *testing.T) {
	t.Parallel()

	feeds := &fakeFeedRepo{feeds: []domain.Feed{{ID: uuid.New(), URL: "https://example.com/feed"}}}
	posts := &fakePostWriter{}
	fetcher := &fakeFetcher{err: errBoom}

	runOnePass(t, newScraper(t, feeds, posts, fetcher, time.Now().UTC()))

	// The feed is still marked so a permanently broken feed cannot starve the
	// rotation.
	assert.Len(t, feeds.markedIDs(), 1)
	assert.Empty(t, posts.posts())
}

func TestScraperSurvivesRepositoryFailure(t *testing.T) {
	t.Parallel()

	feeds := &fakeFeedRepo{nextErr: errBoom}
	posts := &fakePostWriter{}
	fetcher := &fakeFetcher{}

	// Must return cleanly rather than crash: the old loop had no error path
	// other than logging and continuing forever.
	runOnePass(t, newScraper(t, feeds, posts, fetcher, time.Now().UTC()))

	assert.Empty(t, posts.posts())
}

func TestScraperStopsOnContextCancellation(t *testing.T) {
	t.Parallel()

	feeds := &fakeFeedRepo{feeds: []domain.Feed{{ID: uuid.New(), URL: "https://example.com/feed"}}}
	s := scraper.New(feeds, &fakePostWriter{}, &fakeFetcher{}, fixedClock{now: time.Now().UTC()}, &seqIDs{},
		testLogger(), scraper.Options{Concurrency: 1, Interval: 10 * time.Millisecond, RequestTimeout: time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run ignored context cancellation")
	}
}

func TestScraperReturnsImmediatelyOnCancelledContext(t *testing.T) {
	t.Parallel()

	feeds := &fakeFeedRepo{}
	s := newScraper(t, feeds, &fakePostWriter{}, &fakeFetcher{}, time.Now().UTC())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, s.Run(ctx))
	assert.Zero(t, feeds.nextCalls, "an already-cancelled context must do no work")
}

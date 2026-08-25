package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
	"github.com/matteo-campana/rss-aggregator/internal/service"
)

func TestFeedServiceCreate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	id := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	owner := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	repo := &fakeFeedRepo{}
	svc := service.NewFeedService(repo, fixedClock{now: now}, fixedIDs{id: id}, defaultPageSize, maxPageSize)

	feed, err := svc.Create(context.Background(), owner, " Go Blog ", "https://go.dev/blog/feed.atom")
	require.NoError(t, err)

	assert.Equal(t, id, feed.ID)
	assert.Equal(t, "Go Blog", feed.Name)
	assert.Equal(t, "https://go.dev/blog/feed.atom", feed.URL)
	assert.Equal(t, owner, feed.UserID)
	assert.Equal(t, now, feed.CreatedAt)
	assert.Nil(t, feed.LastFetchedAt, "a new feed has never been fetched")
}

func TestFeedServiceCreateRejectsBadURLs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
	}{
		{name: "empty", url: ""},
		{name: "whitespace only", url: "   "},
		{name: "no scheme", url: "go.dev/feed.xml"},
		{name: "file scheme", url: "file:///etc/passwd"},
		{name: "ftp scheme", url: "ftp://example.com/feed.xml"},
		{name: "no host", url: "https://"},
		{name: "control characters", url: "https://exa\x7fmple.com"},
		// Courtesy checks: the dial guard in feedfetch is the real boundary,
		// but the obvious attempts should fail at creation time.
		{name: "localhost", url: "http://localhost:8080/feed.xml"},
		{name: "localhost subdomain", url: "http://api.LOCALHOST/feed.xml"},
		{name: "loopback literal", url: "http://127.0.0.1/feed.xml"},
		{name: "ipv6 loopback literal", url: "http://[::1]/feed.xml"},
		{name: "cloud metadata endpoint", url: "http://169.254.169.254/latest/meta-data/"},
		{name: "ipv4-mapped metadata endpoint", url: "http://[::ffff:169.254.169.254]/latest/meta-data/"},
		{name: "rfc1918 literal", url: "http://10.0.0.5/feed.xml"},
		{name: "ipv6 unique local", url: "http://[fd00::1]/feed.xml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &fakeFeedRepo{}
			svc := service.NewFeedService(repo, fixedClock{}, fixedIDs{}, defaultPageSize, maxPageSize)

			_, err := svc.Create(context.Background(), uuid.New(), "name", tt.url)

			assert.ErrorIs(t, err, domain.ErrInvalidInput)
			assert.Equal(t, domain.Feed{}, repo.lastCreated, "must not reach the repository")
		})
	}
}

func TestFeedServiceCreateRejectsEmptyName(t *testing.T) {
	t.Parallel()

	svc := service.NewFeedService(&fakeFeedRepo{}, fixedClock{}, fixedIDs{}, defaultPageSize, maxPageSize)

	_, err := svc.Create(context.Background(), uuid.New(), "  ", "https://example.com/feed.xml")

	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestFeedServiceCreatePropagatesConflict(t *testing.T) {
	t.Parallel()

	// feeds.url is UNIQUE: registering the same feed twice must surface as a
	// conflict so the handler answers 409.
	repo := &fakeFeedRepo{createErr: domain.ErrConflict}
	svc := service.NewFeedService(repo, fixedClock{}, fixedIDs{}, defaultPageSize, maxPageSize)

	_, err := svc.Create(context.Background(), uuid.New(), "dup", "https://example.com/feed.xml")

	assert.ErrorIs(t, err, domain.ErrConflict)
}

func TestFeedServiceList(t *testing.T) {
	t.Parallel()

	want := []domain.Feed{{ID: uuid.New(), Name: "one"}}
	svc := service.NewFeedService(&fakeFeedRepo{feeds: want}, fixedClock{}, fixedIDs{}, defaultPageSize, maxPageSize)

	got, err := svc.List(context.Background(), 0, 0)

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// The same clamping contract as PostService: this endpoint is public, so the
// bounds are what stop an anonymous caller dumping the whole feeds table.
func TestFeedServiceListClampsPagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		limit      int32
		offset     int32
		wantLimit  int32
		wantOffset int32
	}{
		{name: "zero limit uses the default", limit: 0, wantLimit: defaultPageSize},
		{name: "negative limit uses the default", limit: -5, wantLimit: defaultPageSize},
		{name: "limit above the maximum is capped", limit: 5000, wantLimit: maxPageSize},
		{name: "limit within range is kept", limit: 42, wantLimit: 42},
		{name: "negative offset becomes zero", limit: 10, offset: -1, wantLimit: 10, wantOffset: 0},
		{name: "offset is passed through", limit: 10, offset: 30, wantLimit: 10, wantOffset: 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &fakeFeedRepo{}
			svc := service.NewFeedService(repo, fixedClock{}, fixedIDs{}, defaultPageSize, maxPageSize)

			_, err := svc.List(context.Background(), tt.limit, tt.offset)
			require.NoError(t, err)

			assert.Equal(t, tt.wantLimit, repo.gotLimit)
			assert.Equal(t, tt.wantOffset, repo.gotOffset)
		})
	}
}

func TestFeedServiceListPropagatesError(t *testing.T) {
	t.Parallel()

	svc := service.NewFeedService(&fakeFeedRepo{listErr: errBoom}, fixedClock{}, fixedIDs{}, defaultPageSize, maxPageSize)

	_, err := svc.List(context.Background(), 0, 0)

	assert.ErrorIs(t, err, errBoom)
}

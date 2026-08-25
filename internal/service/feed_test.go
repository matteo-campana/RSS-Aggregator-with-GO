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
	svc := service.NewFeedService(repo, fixedClock{now: now}, fixedIDs{id: id})

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &fakeFeedRepo{}
			svc := service.NewFeedService(repo, fixedClock{}, fixedIDs{})

			_, err := svc.Create(context.Background(), uuid.New(), "name", tt.url)

			assert.ErrorIs(t, err, domain.ErrInvalidInput)
			assert.Equal(t, domain.Feed{}, repo.lastCreated, "must not reach the repository")
		})
	}
}

func TestFeedServiceCreateRejectsEmptyName(t *testing.T) {
	t.Parallel()

	svc := service.NewFeedService(&fakeFeedRepo{}, fixedClock{}, fixedIDs{})

	_, err := svc.Create(context.Background(), uuid.New(), "  ", "https://example.com/feed.xml")

	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestFeedServiceCreatePropagatesConflict(t *testing.T) {
	t.Parallel()

	// feeds.url is UNIQUE: registering the same feed twice must surface as a
	// conflict so the handler answers 409.
	repo := &fakeFeedRepo{createErr: domain.ErrConflict}
	svc := service.NewFeedService(repo, fixedClock{}, fixedIDs{})

	_, err := svc.Create(context.Background(), uuid.New(), "dup", "https://example.com/feed.xml")

	assert.ErrorIs(t, err, domain.ErrConflict)
}

func TestFeedServiceList(t *testing.T) {
	t.Parallel()

	want := []domain.Feed{{ID: uuid.New(), Name: "one"}}
	svc := service.NewFeedService(&fakeFeedRepo{feeds: want}, fixedClock{}, fixedIDs{})

	got, err := svc.List(context.Background())

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestFeedServiceListPropagatesError(t *testing.T) {
	t.Parallel()

	svc := service.NewFeedService(&fakeFeedRepo{listErr: errBoom}, fixedClock{}, fixedIDs{})

	_, err := svc.List(context.Background())

	assert.ErrorIs(t, err, errBoom)
}

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

func TestFeedFollowServiceCreate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	id := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	owner, feedID := uuid.New(), uuid.New()

	repo := &fakeFeedFollowRepo{}
	svc := service.NewFeedFollowService(repo, fixedClock{now: now}, fixedIDs{id: id}, defaultPageSize, maxPageSize)

	follow, err := svc.Create(context.Background(), owner, feedID)
	require.NoError(t, err)

	assert.Equal(t, id, follow.ID)
	assert.Equal(t, owner, follow.UserID)
	assert.Equal(t, feedID, follow.FeedID)
	// The old handler_feed_follows.go used local time here while every other
	// path used UTC; the injected clock removes that inconsistency.
	assert.Equal(t, now, follow.CreatedAt)
	assert.Equal(t, time.UTC, follow.CreatedAt.Location())
}

func TestFeedFollowServiceCreateRejectsNilFeedID(t *testing.T) {
	t.Parallel()

	repo := &fakeFeedFollowRepo{}
	svc := service.NewFeedFollowService(repo, fixedClock{}, fixedIDs{}, defaultPageSize, maxPageSize)

	_, err := svc.Create(context.Background(), uuid.New(), uuid.Nil)

	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.Equal(t, domain.FeedFollow{}, repo.lastCreated)
}

func TestFeedFollowServiceCreateSurfacesUnknownFeed(t *testing.T) {
	t.Parallel()

	// A foreign-key violation is translated to ErrNotFound by the storage
	// layer, so following a feed that does not exist answers 404.
	repo := &fakeFeedFollowRepo{createErr: domain.ErrNotFound}
	svc := service.NewFeedFollowService(repo, fixedClock{}, fixedIDs{}, defaultPageSize, maxPageSize)

	_, err := svc.Create(context.Background(), uuid.New(), uuid.New())

	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestFeedFollowServiceDelete(t *testing.T) {
	t.Parallel()

	id, owner := uuid.New(), uuid.New()
	repo := &fakeFeedFollowRepo{}
	svc := service.NewFeedFollowService(repo, fixedClock{}, fixedIDs{}, defaultPageSize, maxPageSize)

	require.NoError(t, svc.Delete(context.Background(), id, owner))

	assert.Equal(t, id, repo.deletedID)
	assert.Equal(t, owner, repo.deletedOwner, "deletion must be scoped to the owner")
}

func TestFeedFollowServiceDeleteReportsMissing(t *testing.T) {
	t.Parallel()

	repo := &fakeFeedFollowRepo{deleteErr: domain.ErrNotFound}
	svc := service.NewFeedFollowService(repo, fixedClock{}, fixedIDs{}, defaultPageSize, maxPageSize)

	err := svc.Delete(context.Background(), uuid.New(), uuid.New())

	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestFeedFollowServiceDeleteRejectsNilID(t *testing.T) {
	t.Parallel()

	svc := service.NewFeedFollowService(&fakeFeedFollowRepo{}, fixedClock{}, fixedIDs{}, defaultPageSize, maxPageSize)

	err := svc.Delete(context.Background(), uuid.Nil, uuid.New())

	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestFeedFollowServiceListByUser(t *testing.T) {
	t.Parallel()

	want := []domain.FeedFollow{{ID: uuid.New()}}
	svc := service.NewFeedFollowService(&fakeFeedFollowRepo{follows: want}, fixedClock{}, fixedIDs{}, defaultPageSize, maxPageSize)

	got, err := svc.ListByUser(context.Background(), uuid.New(), 0, 0)

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

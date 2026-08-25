package service_test

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// errBoom stands in for any unexpected infrastructure failure.
var errBoom = errors.New("boom")

// Pagination bounds shared by the paginated-listing tests.
const (
	defaultPageSize int32 = 10
	maxPageSize     int32 = 100
)

// fixedClock returns a constant time so assertions can compare exactly.
type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

// fixedIDs hands out a predetermined identifier.
type fixedIDs struct{ id uuid.UUID }

func (g fixedIDs) NewID() uuid.UUID { return g.id }

// fixedKeys hands out a predetermined API key, or an error.
type fixedKeys struct {
	key string
	err error
}

func (g fixedKeys) Generate() (string, error) { return g.key, g.err }

type fakeUserRepo struct {
	byKey     map[string]domain.User
	createErr error
	getErr    error

	lastCreated domain.User
}

func (r *fakeUserRepo) Create(_ context.Context, u domain.User) (domain.User, error) {
	if r.createErr != nil {
		return domain.User{}, r.createErr
	}
	r.lastCreated = u
	return u, nil
}

func (r *fakeUserRepo) GetByAPIKey(_ context.Context, apiKey string) (domain.User, error) {
	if r.getErr != nil {
		return domain.User{}, r.getErr
	}
	u, ok := r.byKey[apiKey]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return u, nil
}

type fakeFeedRepo struct {
	feeds     []domain.Feed
	createErr error
	listErr   error

	lastCreated domain.Feed
	gotLimit    int32
	gotOffset   int32
}

func (r *fakeFeedRepo) Create(_ context.Context, f domain.Feed) (domain.Feed, error) {
	if r.createErr != nil {
		return domain.Feed{}, r.createErr
	}
	r.lastCreated = f
	return f, nil
}

func (r *fakeFeedRepo) List(_ context.Context, limit, offset int32) ([]domain.Feed, error) {
	r.gotLimit, r.gotOffset = limit, offset
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.feeds, nil
}

type fakeFeedFollowRepo struct {
	follows   []domain.FeedFollow
	createErr error
	listErr   error
	deleteErr error

	lastCreated  domain.FeedFollow
	deletedID    uuid.UUID
	deletedOwner uuid.UUID
}

func (r *fakeFeedFollowRepo) Create(_ context.Context, ff domain.FeedFollow) (domain.FeedFollow, error) {
	if r.createErr != nil {
		return domain.FeedFollow{}, r.createErr
	}
	r.lastCreated = ff
	return ff, nil
}

func (r *fakeFeedFollowRepo) ListByUser(_ context.Context, _ uuid.UUID) ([]domain.FeedFollow, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.follows, nil
}

func (r *fakeFeedFollowRepo) Delete(_ context.Context, id, userID uuid.UUID) error {
	r.deletedID, r.deletedOwner = id, userID
	return r.deleteErr
}

type fakePostRepo struct {
	posts   []domain.Post
	listErr error

	gotLimit  int32
	gotOffset int32
}

func (r *fakePostRepo) ListForUser(_ context.Context, _ uuid.UUID, limit, offset int32) ([]domain.Post, error) {
	r.gotLimit, r.gotOffset = limit, offset
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.posts, nil
}

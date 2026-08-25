package http_test

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

var errBoom = errors.New("boom")

type fakeUserService struct {
	user       domain.User
	createErr  error
	authErr    error
	gotName    string
	gotAPIKey  string
	authCalled bool
}

func (s *fakeUserService) Create(_ context.Context, name string) (domain.User, error) {
	s.gotName = name
	if s.createErr != nil {
		return domain.User{}, s.createErr
	}
	return s.user, nil
}

func (s *fakeUserService) Authenticate(_ context.Context, apiKey string) (domain.User, error) {
	s.authCalled = true
	s.gotAPIKey = apiKey
	if s.authErr != nil {
		return domain.User{}, s.authErr
	}
	return s.user, nil
}

type fakeFeedService struct {
	feed      domain.Feed
	feeds     []domain.Feed
	createErr error
	listErr   error

	gotName, gotURL string
	gotUserID       uuid.UUID
	gotLimit        int32
	gotOffset       int32
}

func (s *fakeFeedService) Create(_ context.Context, userID uuid.UUID, name, url string) (domain.Feed, error) {
	s.gotUserID, s.gotName, s.gotURL = userID, name, url
	if s.createErr != nil {
		return domain.Feed{}, s.createErr
	}
	return s.feed, nil
}

func (s *fakeFeedService) List(_ context.Context, limit, offset int32) ([]domain.Feed, error) {
	s.gotLimit, s.gotOffset = limit, offset
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.feeds, nil
}

type fakeFeedFollowService struct {
	follow    domain.FeedFollow
	follows   []domain.FeedFollow
	createErr error
	listErr   error
	deleteErr error

	gotFeedID uuid.UUID
	gotID     uuid.UUID
	gotUserID uuid.UUID
}

func (s *fakeFeedFollowService) Create(_ context.Context, userID, feedID uuid.UUID) (domain.FeedFollow, error) {
	s.gotUserID, s.gotFeedID = userID, feedID
	if s.createErr != nil {
		return domain.FeedFollow{}, s.createErr
	}
	return s.follow, nil
}

func (s *fakeFeedFollowService) ListByUser(_ context.Context, userID uuid.UUID) ([]domain.FeedFollow, error) {
	s.gotUserID = userID
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.follows, nil
}

func (s *fakeFeedFollowService) Delete(_ context.Context, id, userID uuid.UUID) error {
	s.gotID, s.gotUserID = id, userID
	return s.deleteErr
}

type fakePostService struct {
	posts   []domain.Post
	listErr error

	gotLimit  int32
	gotOffset int32
	gotUserID uuid.UUID
}

func (s *fakePostService) ListForUser(_ context.Context, userID uuid.UUID, limit, offset int32) ([]domain.Post, error) {
	s.gotUserID, s.gotLimit, s.gotOffset = userID, limit, offset
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.posts, nil
}

package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// FeedFollowService implements the rules for following and unfollowing feeds.
type FeedFollowService struct {
	repo  FeedFollowRepository
	clock Clock
	ids   IDGenerator
}

// NewFeedFollowService wires a FeedFollowService with its dependencies.
func NewFeedFollowService(repo FeedFollowRepository, clock Clock, ids IDGenerator) *FeedFollowService {
	return &FeedFollowService{repo: repo, clock: clock, ids: ids}
}

// Create makes the user follow a feed.
func (s *FeedFollowService) Create(ctx context.Context, userID, feedID uuid.UUID) (domain.FeedFollow, error) {
	if feedID == uuid.Nil {
		return domain.FeedFollow{}, domain.NewValidationError("feed_id", "must be a valid UUID")
	}

	now := s.clock.Now()
	follow, err := s.repo.Create(ctx, domain.FeedFollow{
		ID:        s.ids.NewID(),
		CreatedAt: now,
		UpdatedAt: now,
		UserID:    userID,
		FeedID:    feedID,
	})
	if err != nil {
		return domain.FeedFollow{}, fmt.Errorf("create feed follow: %w", err)
	}
	return follow, nil
}

// ListByUser returns the feeds the user follows.
func (s *FeedFollowService) ListByUser(ctx context.Context, userID uuid.UUID) ([]domain.FeedFollow, error) {
	follows, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list feed follows: %w", err)
	}
	return follows, nil
}

// Delete removes one of the user's follows.
func (s *FeedFollowService) Delete(ctx context.Context, id, userID uuid.UUID) error {
	if id == uuid.Nil {
		return domain.NewValidationError("feed_follow_id", "must be a valid UUID")
	}
	if err := s.repo.Delete(ctx, id, userID); err != nil {
		return fmt.Errorf("delete feed follow: %w", err)
	}
	return nil
}

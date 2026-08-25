package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// FeedFollowService implements the rules for following and unfollowing feeds.
type FeedFollowService struct {
	repo            FeedFollowRepository
	clock           Clock
	ids             IDGenerator
	defaultPageSize int32
	maxPageSize     int32
}

// NewFeedFollowService wires a FeedFollowService with its dependencies and
// pagination bounds.
func NewFeedFollowService(
	repo FeedFollowRepository,
	clock Clock,
	ids IDGenerator,
	defaultPageSize, maxPageSize int32,
) *FeedFollowService {
	return &FeedFollowService{
		repo:            repo,
		clock:           clock,
		ids:             ids,
		defaultPageSize: defaultPageSize,
		maxPageSize:     maxPageSize,
	}
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

// ListByUser returns a page of the feeds the user follows.
func (s *FeedFollowService) ListByUser(
	ctx context.Context,
	userID uuid.UUID,
	limit, offset int32,
) ([]domain.FeedFollow, error) {
	limit, offset, err := clampPage(limit, offset, s.defaultPageSize, s.maxPageSize)
	if err != nil {
		return nil, err
	}

	follows, err := s.repo.ListByUser(ctx, userID, limit, offset)
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

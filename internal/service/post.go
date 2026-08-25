package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// PostService reads the posts of the feeds a user follows.
type PostService struct {
	repo            PostRepository
	defaultPageSize int32
	maxPageSize     int32
}

// NewPostService wires a PostService with its pagination bounds.
func NewPostService(repo PostRepository, defaultPageSize, maxPageSize int32) *PostService {
	return &PostService{
		repo:            repo,
		defaultPageSize: defaultPageSize,
		maxPageSize:     maxPageSize,
	}
}

// ListForUser returns a page of posts, newest first.
//
// The page size was previously hard-coded to 10 in the handler; it is now
// caller-supplied and clamped here so no client can ask for an unbounded scan.
func (s *PostService) ListForUser(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]domain.Post, error) {
	limit, offset = s.clamp(limit, offset)

	posts, err := s.repo.ListForUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list posts: %w", err)
	}
	return posts, nil
}

func (s *PostService) clamp(limit, offset int32) (int32, int32) {
	switch {
	case limit <= 0:
		limit = s.defaultPageSize
	case limit > s.maxPageSize:
		limit = s.maxPageSize
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

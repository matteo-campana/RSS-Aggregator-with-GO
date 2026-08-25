package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// FeedService implements the rules for registering and listing feeds.
type FeedService struct {
	repo            FeedRepository
	clock           Clock
	ids             IDGenerator
	defaultPageSize int32
	maxPageSize     int32
}

// NewFeedService wires a FeedService with its dependencies and pagination bounds.
func NewFeedService(
	repo FeedRepository,
	clock Clock,
	ids IDGenerator,
	defaultPageSize, maxPageSize int32,
) *FeedService {
	return &FeedService{
		repo:            repo,
		clock:           clock,
		ids:             ids,
		defaultPageSize: defaultPageSize,
		maxPageSize:     maxPageSize,
	}
}

// Create registers a feed owned by the given user.
func (s *FeedService) Create(ctx context.Context, userID uuid.UUID, name, rawURL string) (domain.Feed, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return domain.Feed{}, domain.NewValidationError("name", "must not be empty")
	case len(name) > maxNameLength:
		return domain.Feed{}, domain.NewValidationError("name", fmt.Sprintf("must be at most %d characters", maxNameLength))
	}

	feedURL, err := validateFeedURL(rawURL)
	if err != nil {
		return domain.Feed{}, err
	}

	now := s.clock.Now()
	feed, err := s.repo.Create(ctx, domain.Feed{
		ID:        s.ids.NewID(),
		CreatedAt: now,
		UpdatedAt: now,
		Name:      name,
		URL:       feedURL,
		UserID:    userID,
	})
	if err != nil {
		return domain.Feed{}, fmt.Errorf("create feed: %w", err)
	}
	return feed, nil
}

// List returns a page of registered feeds, newest first.
//
// This endpoint is public and previously returned the whole table, so the
// response grew without bound as feeds were added.
func (s *FeedService) List(ctx context.Context, limit, offset int32) ([]domain.Feed, error) {
	limit, offset = clampPage(limit, offset, s.defaultPageSize, s.maxPageSize)

	feeds, err := s.repo.List(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list feeds: %w", err)
	}
	return feeds, nil
}

// validateFeedURL rejects anything the scraper could not or should not fetch.
// Restricting the scheme also stops the scraper being pointed at file:// or
// other local schemes by an API client.
func validateFeedURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", domain.NewValidationError("url", "must not be empty")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", domain.NewValidationError("url", "must be a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", domain.NewValidationError("url", "must use the http or https scheme")
	}
	if parsed.Host == "" {
		return "", domain.NewValidationError("url", "must include a host")
	}
	return parsed.String(), nil
}

package service

import (
	"context"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"unicode/utf8"

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
	case utf8.RuneCountInString(name) > maxNameLength:
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
	limit, offset, err := clampPage(limit, offset, s.defaultPageSize, s.maxPageSize)
	if err != nil {
		return nil, err
	}

	feeds, err := s.repo.List(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list feeds: %w", err)
	}
	return feeds, nil
}

// FindByURL returns the feed registered under a URL.
//
// feeds.url is UNIQUE, so registering a feed someone else already added answers
// 409 with no id in the body. This gives the client the forward path it needs:
// look the feed up, then follow it.
func (s *FeedService) FindByURL(ctx context.Context, rawURL string) (domain.Feed, error) {
	feedURL, err := validateFeedURL(rawURL)
	if err != nil {
		return domain.Feed{}, err
	}

	feed, err := s.repo.GetByURL(ctx, feedURL)
	if err != nil {
		return domain.Feed{}, fmt.Errorf("find feed by url: %w", err)
	}
	return feed, nil
}

// validateFeedURL rejects anything the scraper could not or should not fetch.
//
// The host checks here are a courtesy, not a security boundary: a hostname can
// resolve to anything, and can resolve differently a second later. What
// actually keeps user-supplied feed URLs off the internal network is the dial
// guard in the feedfetch package, which inspects the resolved address of every
// connection, redirects included. This function only turns the obvious attempts
// into an immediate 400 instead of a fetch that fails silently a minute later.
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

	hostname := parsed.Hostname()
	if hostname == "" {
		return "", domain.NewValidationError("url", "must include a host")
	}
	if strings.EqualFold(hostname, "localhost") || strings.HasSuffix(strings.ToLower(hostname), ".localhost") {
		return "", domain.NewValidationError("url", "must not point at the local machine")
	}
	if isReservedLiteral(hostname) {
		return "", domain.NewValidationError("url", "must not point at a private or reserved address")
	}

	return parsed.String(), nil
}

// isReservedLiteral reports whether the host is a literal IP in a range the
// scraper refuses to dial.
func isReservedLiteral(hostname string) bool {
	addr, err := netip.ParseAddr(hostname)
	if err != nil {
		return false
	}
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified()
}

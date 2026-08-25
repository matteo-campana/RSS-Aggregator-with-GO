// Package domain holds the core models and error taxonomy of the application.
//
// It is deliberately dependency-free: nothing in here may import a database
// driver, an HTTP framework or a feed parser. Every other package depends on
// this one, never the other way round.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// User is an account that owns feeds and follows them.
type User struct {
	ID        uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
	Name      string
	APIKey    string
}

// Feed is an RSS/Atom source registered by a user.
type Feed struct {
	ID        uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
	Name      string
	URL       string
	UserID    uuid.UUID
	// LastFetchedAt is nil until the scraper has fetched the feed at least once.
	LastFetchedAt *time.Time
}

// FeedFollow records that a user follows a feed.
type FeedFollow struct {
	ID        uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
	UserID    uuid.UUID
	FeedID    uuid.UUID
}

// Post is a single item scraped from a feed.
type Post struct {
	ID          uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Title       *string
	Description *string
	PublishedAt time.Time
	URL         string
	FeedID      uuid.UUID
}

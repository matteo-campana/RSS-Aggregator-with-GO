// Package service holds the application rules.
//
// Every dependency it needs is declared here as an interface and satisfied by
// an adapter elsewhere, so this package imports no database driver and no HTTP
// framework. The interfaces are deliberately narrow: a consumer that only
// reads posts must not be handed a type that can also delete feeds.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// UserRepository persists users.
type UserRepository interface {
	Create(ctx context.Context, u domain.User) (domain.User, error)
	GetByAPIKey(ctx context.Context, apiKey string) (domain.User, error)
}

// FeedRepository persists feeds. The scraper declares its own, different
// interface: it needs scheduling operations, not creation.
type FeedRepository interface {
	Create(ctx context.Context, f domain.Feed) (domain.Feed, error)
	List(ctx context.Context, limit, offset int32) ([]domain.Feed, error)
}

// FeedFollowRepository persists the user-to-feed relation.
type FeedFollowRepository interface {
	Create(ctx context.Context, ff domain.FeedFollow) (domain.FeedFollow, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]domain.FeedFollow, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
}

// PostRepository reads posts.
type PostRepository interface {
	ListForUser(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]domain.Post, error)
}

// Clock supplies the current time. Injecting it keeps services deterministic
// under test without patching globals.
type Clock interface {
	Now() time.Time
}

// IDGenerator supplies new entity identifiers.
type IDGenerator interface {
	NewID() uuid.UUID
}

// APIKeyGenerator supplies new API keys.
type APIKeyGenerator interface {
	Generate() (string, error)
}

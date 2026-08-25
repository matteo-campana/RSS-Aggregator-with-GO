package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
	"github.com/matteo-campana/rss-aggregator/internal/storage/postgres/sqlc"
)

// FeedRepository stores feeds in PostgreSQL.
//
// It satisfies two different, deliberately narrow interfaces: service.FeedRepository
// (Create/List) and scraper.FeedRepository (NextToFetch/MarkFetched). Neither
// consumer can reach the other's operations — one concrete type, two segregated
// views of it.
type FeedRepository struct {
	q sqlc.Querier
}

// NewFeedRepository builds a FeedRepository over the generated queries.
func NewFeedRepository(q sqlc.Querier) *FeedRepository {
	return &FeedRepository{q: q}
}

// Create inserts a feed.
func (r *FeedRepository) Create(ctx context.Context, f domain.Feed) (domain.Feed, error) {
	row, err := r.q.CreateFeed(ctx, sqlc.CreateFeedParams{
		ID:        f.ID,
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
		Name:      f.Name,
		Url:       f.URL,
		UserID:    f.UserID,
	})
	if err != nil {
		return domain.Feed{}, translate(err)
	}
	return toDomainFeed(row), nil
}

// List returns a page of feeds, newest first.
func (r *FeedRepository) List(ctx context.Context, limit, offset int32) ([]domain.Feed, error) {
	rows, err := r.q.GetFeeds(ctx, sqlc.GetFeedsParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, translate(err)
	}
	return toDomainFeeds(rows), nil
}

// NextToFetch returns the least recently fetched feeds that are due,
// never-fetched first.
func (r *FeedRepository) NextToFetch(
	ctx context.Context,
	limit int32,
	notFetchedSince time.Time,
) ([]domain.Feed, error) {
	since := notFetchedSince.UTC()
	rows, err := r.q.GetNextFeedsToFetch(ctx, sqlc.GetNextFeedsToFetchParams{
		Limit:         limit,
		LastFetchedAt: &since,
	})
	if err != nil {
		return nil, translate(err)
	}
	return toDomainFeeds(rows), nil
}

// MarkFetched records that a feed has just been fetched.
//
// The timestamp is supplied by the caller's clock rather than by the database's
// NOW(), which keeps the scraper deterministic under test.
func (r *FeedRepository) MarkFetched(ctx context.Context, id uuid.UUID, at time.Time) error {
	utcAt := at.UTC()
	_, err := r.q.MarkFeedAsFetched(ctx, sqlc.MarkFeedAsFetchedParams{
		ID:            id,
		LastFetchedAt: &utcAt,
	})
	return translate(err)
}

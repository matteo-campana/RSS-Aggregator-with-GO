package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
	"github.com/matteo-campana/rss-aggregator/internal/storage/postgres/sqlc"
)

// FeedFollowRepository stores feed follows in PostgreSQL.
type FeedFollowRepository struct {
	q sqlc.Querier
}

// NewFeedFollowRepository builds a FeedFollowRepository over the generated queries.
func NewFeedFollowRepository(q sqlc.Querier) *FeedFollowRepository {
	return &FeedFollowRepository{q: q}
}

// Create inserts a feed follow.
//
// A follow that already exists surfaces as domain.ErrConflict, and a feed id
// that does not exist as domain.ErrNotFound, both via the SQLSTATE translation
// in translate().
func (r *FeedFollowRepository) Create(ctx context.Context, ff domain.FeedFollow) (domain.FeedFollow, error) {
	row, err := r.q.CreateFeedFollow(ctx, sqlc.CreateFeedFollowParams{
		ID:        ff.ID,
		CreatedAt: ff.CreatedAt,
		UpdatedAt: ff.UpdatedAt,
		UserID:    ff.UserID,
		FeedID:    ff.FeedID,
	})
	if err != nil {
		return domain.FeedFollow{}, translate(err)
	}
	return toDomainFeedFollow(row), nil
}

// ListByUser returns the follows belonging to a user.
func (r *FeedFollowRepository) ListByUser(
	ctx context.Context,
	userID uuid.UUID,
	limit, offset int32,
) ([]domain.FeedFollow, error) {
	rows, err := r.q.GetFeedFollows(ctx, sqlc.GetFeedFollowsParams{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, translate(err)
	}
	return toDomainFeedFollows(rows), nil
}

// Delete removes a follow owned by the given user, reporting
// domain.ErrNotFound when nothing matched.
func (r *FeedFollowRepository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	affected, err := r.q.DeleteFeedFollow(ctx, sqlc.DeleteFeedFollowParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		return translate(err)
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

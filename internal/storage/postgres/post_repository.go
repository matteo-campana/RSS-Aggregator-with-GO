package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
	"github.com/matteo-campana/rss-aggregator/internal/storage/postgres/sqlc"
)

// PostRepository stores and reads posts in PostgreSQL.
type PostRepository struct {
	q sqlc.Querier
}

// NewPostRepository builds a PostRepository over the generated queries.
func NewPostRepository(q sqlc.Querier) *PostRepository {
	return &PostRepository{q: q}
}

// Create inserts a scraped post.
//
// Only the scraper writes posts and it has no use for the inserted row, so the
// created record is intentionally not returned.
func (r *PostRepository) Create(ctx context.Context, p domain.Post) error {
	// The columns are TIMESTAMP without time zone, so the write path normalises
	// explicitly rather than relying on the Clock happening to return UTC.
	_, err := r.q.CreatePost(ctx, sqlc.CreatePostParams{
		ID:          p.ID,
		CreatedAt:   p.CreatedAt.UTC(),
		UpdatedAt:   p.UpdatedAt.UTC(),
		Title:       p.Title,
		Description: p.Description,
		PublishedAt: p.PublishedAt.UTC(),
		Url:         p.URL,
		FeedID:      p.FeedID,
	})
	return translate(err)
}

// ListForUser returns the most recently published posts across the feeds the
// user follows.
func (r *PostRepository) ListForUser(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]domain.Post, error) {
	rows, err := r.q.GetPostsForUser(ctx, sqlc.GetPostsForUserParams{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, translate(err)
	}
	return toDomainPosts(rows), nil
}

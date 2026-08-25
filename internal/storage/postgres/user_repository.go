package postgres

import (
	"context"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
	"github.com/matteo-campana/rss-aggregator/internal/storage/postgres/sqlc"
)

// UserRepository stores users in PostgreSQL.
type UserRepository struct {
	q sqlc.Querier
}

// NewUserRepository builds a UserRepository over the generated queries.
func NewUserRepository(q sqlc.Querier) *UserRepository {
	return &UserRepository{q: q}
}

// Create inserts a user.
func (r *UserRepository) Create(ctx context.Context, u domain.User) (domain.User, error) {
	row, err := r.q.CreateUser(ctx, sqlc.CreateUserParams{
		ID:        u.ID,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
		Name:      u.Name,
		ApiKey:    u.APIKey,
	})
	if err != nil {
		return domain.User{}, translate(err)
	}
	return toDomainUser(row), nil
}

// GetByAPIKey looks a user up by API key, reporting domain.ErrNotFound when no
// row matches.
func (r *UserRepository) GetByAPIKey(ctx context.Context, apiKey string) (domain.User, error) {
	row, err := r.q.GetUserByApiKey(ctx, apiKey)
	if err != nil {
		return domain.User{}, translate(err)
	}
	return toDomainUser(row), nil
}

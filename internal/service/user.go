package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// maxNameLength counts characters, not bytes: len() would reject a 100-character
// Japanese name as "more than 255 characters", which is both false and
// unactionable. The column is TEXT, so nothing forces a byte limit.
const maxNameLength = 255

// UserService implements the user-facing rules: registration and
// authentication by API key.
type UserService struct {
	repo  UserRepository
	keys  APIKeyGenerator
	clock Clock
	ids   IDGenerator
}

// NewUserService wires a UserService with its dependencies.
func NewUserService(repo UserRepository, keys APIKeyGenerator, clock Clock, ids IDGenerator) *UserService {
	return &UserService{repo: repo, keys: keys, clock: clock, ids: ids}
}

// Create registers a new user and issues an API key for it.
func (s *UserService) Create(ctx context.Context, name string) (domain.User, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return domain.User{}, domain.NewValidationError("name", "must not be empty")
	case utf8.RuneCountInString(name) > maxNameLength:
		return domain.User{}, domain.NewValidationError("name", fmt.Sprintf("must be at most %d characters", maxNameLength))
	}

	key, err := s.keys.Generate()
	if err != nil {
		return domain.User{}, fmt.Errorf("issue api key: %w", err)
	}

	now := s.clock.Now()
	user, err := s.repo.Create(ctx, domain.User{
		ID:        s.ids.NewID(),
		CreatedAt: now,
		UpdatedAt: now,
		Name:      name,
		APIKey:    key,
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

// Authenticate resolves an API key to its user.
//
// A key that matches nothing is reported as ErrUnauthorized rather than
// ErrNotFound, so the transport layer answers 401 and never reveals whether a
// given key exists.
func (s *UserService) Authenticate(ctx context.Context, apiKey string) (domain.User, error) {
	if strings.TrimSpace(apiKey) == "" {
		return domain.User{}, fmt.Errorf("%w: empty API key", domain.ErrUnauthorized)
	}

	user, err := s.repo.GetByAPIKey(ctx, apiKey)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.User{}, fmt.Errorf("%w: unknown API key", domain.ErrUnauthorized)
		}
		return domain.User{}, fmt.Errorf("authenticate: %w", err)
	}
	return user, nil
}

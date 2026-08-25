package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
	"github.com/matteo-campana/rss-aggregator/internal/service"
)

func TestUserServiceCreate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	repo := &fakeUserRepo{}
	svc := service.NewUserService(repo, fixedKeys{key: "generated-key"}, fixedClock{now: now}, fixedIDs{id: id})

	user, err := svc.Create(context.Background(), "  Ada  ")
	require.NoError(t, err)

	assert.Equal(t, id, user.ID)
	assert.Equal(t, "Ada", user.Name, "name should be trimmed")
	assert.Equal(t, "generated-key", user.APIKey, "key must come from the injected generator")
	assert.Equal(t, now, user.CreatedAt)
	assert.Equal(t, now, user.UpdatedAt)
	assert.Equal(t, user, repo.lastCreated)
}

func TestUserServiceCreateValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "whitespace only", input: "   "},
		{name: "too long", input: strings.Repeat("a", 256)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &fakeUserRepo{}
			svc := service.NewUserService(repo, fixedKeys{key: "k"}, fixedClock{}, fixedIDs{})

			_, err := svc.Create(context.Background(), tt.input)

			assert.ErrorIs(t, err, domain.ErrInvalidInput)
			assert.Equal(t, domain.User{}, repo.lastCreated, "must not reach the repository")
		})
	}
}

// The limit counts characters, not bytes: counting bytes rejected this name at
// 300 bytes while telling the caller it exceeded 255 characters.
func TestUserServiceCreateAcceptsMultiByteName(t *testing.T) {
	t.Parallel()

	repo := &fakeUserRepo{}
	svc := service.NewUserService(repo, fixedKeys{key: "k"}, fixedClock{}, fixedIDs{})

	name := strings.Repeat("あ", 100) // 100 characters, 300 bytes

	user, err := svc.Create(context.Background(), name)

	require.NoError(t, err)
	assert.Equal(t, name, user.Name)
}

func TestUserServiceCreatePropagatesFailures(t *testing.T) {
	t.Parallel()

	t.Run("key generation fails", func(t *testing.T) {
		t.Parallel()

		svc := service.NewUserService(&fakeUserRepo{}, fixedKeys{err: errBoom}, fixedClock{}, fixedIDs{})

		_, err := svc.Create(context.Background(), "Ada")

		assert.ErrorIs(t, err, errBoom)
	})

	t.Run("repository fails", func(t *testing.T) {
		t.Parallel()

		svc := service.NewUserService(&fakeUserRepo{createErr: errBoom}, fixedKeys{key: "k"}, fixedClock{}, fixedIDs{})

		_, err := svc.Create(context.Background(), "Ada")

		assert.ErrorIs(t, err, errBoom)
	})
}

func TestUserServiceAuthenticate(t *testing.T) {
	t.Parallel()

	known := domain.User{ID: uuid.New(), Name: "Ada", APIKey: "good-key"}
	repo := &fakeUserRepo{byKey: map[string]domain.User{"good-key": known}}
	svc := service.NewUserService(repo, fixedKeys{}, fixedClock{}, fixedIDs{})

	t.Run("known key resolves", func(t *testing.T) {
		t.Parallel()

		user, err := svc.Authenticate(context.Background(), "good-key")

		require.NoError(t, err)
		assert.Equal(t, known, user)
	})

	t.Run("unknown key is unauthorized, not not-found", func(t *testing.T) {
		t.Parallel()

		_, err := svc.Authenticate(context.Background(), "bad-key")

		// Reporting 404 here would tell an attacker which keys exist.
		assert.ErrorIs(t, err, domain.ErrUnauthorized)
		assert.NotErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("empty key is rejected without touching the repository", func(t *testing.T) {
		t.Parallel()

		strict := &fakeUserRepo{getErr: errBoom}
		svc := service.NewUserService(strict, fixedKeys{}, fixedClock{}, fixedIDs{})

		_, err := svc.Authenticate(context.Background(), "  ")

		assert.ErrorIs(t, err, domain.ErrUnauthorized)
		assert.NotErrorIs(t, err, errBoom)
	})

	t.Run("infrastructure failure is not downgraded to unauthorized", func(t *testing.T) {
		t.Parallel()

		broken := service.NewUserService(&fakeUserRepo{getErr: errBoom}, fixedKeys{}, fixedClock{}, fixedIDs{})

		_, err := broken.Authenticate(context.Background(), "any")

		assert.ErrorIs(t, err, errBoom)
		assert.NotErrorIs(t, err, domain.ErrUnauthorized)
	})
}

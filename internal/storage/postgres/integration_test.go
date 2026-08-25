//go:build integration

// These tests exercise the real SQL against a real PostgreSQL instance. They
// are behind the `integration` build tag and skip themselves when TEST_DB_URL
// is unset, so `go test ./...` stays runnable without a database.
//
//	TEST_DB_URL=postgres://... go test ./... -tags=integration
//
// The database must already have the goose migrations applied.
package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
	"github.com/matteo-campana/rss-aggregator/internal/storage/postgres"
	"github.com/matteo-campana/rss-aggregator/internal/storage/postgres/sqlc"
)

type repos struct {
	users   *postgres.UserRepository
	feeds   *postgres.FeedRepository
	follows *postgres.FeedFollowRepository
	posts   *postgres.PostRepository
	pool    *pgxpool.Pool
}

func setup(t *testing.T) (context.Context, *repos) {
	t.Helper()

	dsn := os.Getenv("TEST_DB_URL")
	if dsn == "" {
		t.Skip("TEST_DB_URL is not set; skipping database-backed tests")
	}

	ctx := t.Context()

	pool, err := postgres.NewPool(ctx, dsn, postgres.PoolOptions{MaxConns: 4})
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	q := sqlc.New(pool)
	return ctx, &repos{
		users:   postgres.NewUserRepository(q),
		feeds:   postgres.NewFeedRepository(q),
		follows: postgres.NewFeedFollowRepository(q),
		posts:   postgres.NewPostRepository(q),
		pool:    pool,
	}
}

// newUser inserts a user and schedules its removal. The ON DELETE CASCADE
// chain removes the feeds, follows and posts created under it.
func newUser(ctx context.Context, t *testing.T, r *repos) domain.User {
	t.Helper()

	now := time.Now().UTC().Truncate(time.Microsecond)
	user, err := r.users.Create(ctx, domain.User{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-" + uuid.NewString(),
		APIKey:    uuid.NewString() + uuid.NewString(),
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = r.pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})
	return user
}

func newFeed(ctx context.Context, t *testing.T, r *repos, owner domain.User) domain.Feed {
	t.Helper()

	now := time.Now().UTC().Truncate(time.Microsecond)
	feed, err := r.feeds.Create(ctx, domain.Feed{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "feed",
		URL:       "https://example.com/" + uuid.NewString(),
		UserID:    owner.ID,
	})
	require.NoError(t, err)
	return feed
}

func TestUserRoundTrip(t *testing.T) {
	ctx, r := setup(t)
	user := newUser(ctx, t, r)

	got, err := r.users.GetByAPIKey(ctx, user.APIKey)
	require.NoError(t, err)

	assert.Equal(t, user.ID, got.ID)
	assert.Equal(t, user.Name, got.Name)
	assert.Equal(t, time.UTC, got.CreatedAt.Location())
}

func TestGetUserByAPIKeyReportsNotFound(t *testing.T) {
	ctx, r := setup(t)

	_, err := r.users.GetByAPIKey(ctx, "definitely-not-a-key")

	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDuplicateFeedURLIsConflict(t *testing.T) {
	ctx, r := setup(t)
	user := newUser(ctx, t, r)
	feed := newFeed(ctx, t, r, user)

	now := time.Now().UTC()
	_, err := r.feeds.Create(ctx, domain.Feed{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now,
		Name: "dup", URL: feed.URL, UserID: user.ID,
	})

	// feeds.url is UNIQUE; this is SQLSTATE 23505.
	assert.ErrorIs(t, err, domain.ErrConflict)
}

func TestFeedFetchScheduling(t *testing.T) {
	ctx, r := setup(t)
	user := newUser(ctx, t, r)
	feed := newFeed(ctx, t, r, user)

	assert.Nil(t, feed.LastFetchedAt, "a new feed has never been fetched")

	at := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, r.feeds.MarkFetched(ctx, feed.ID, at))

	feeds, err := r.feeds.List(ctx)
	require.NoError(t, err)

	var found *domain.Feed
	for i := range feeds {
		if feeds[i].ID == feed.ID {
			found = &feeds[i]
			break
		}
	}
	require.NotNil(t, found)
	require.NotNil(t, found.LastFetchedAt)
	assert.WithinDuration(t, at, *found.LastFetchedAt, time.Second)
}

func TestFollowingAnUnknownFeedIsNotFound(t *testing.T) {
	ctx, r := setup(t)
	user := newUser(ctx, t, r)

	now := time.Now().UTC()
	_, err := r.follows.Create(ctx, domain.FeedFollow{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now,
		UserID: user.ID, FeedID: uuid.New(),
	})

	// Foreign-key violation, SQLSTATE 23503.
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDuplicateFollowIsConflict(t *testing.T) {
	ctx, r := setup(t)
	user := newUser(ctx, t, r)
	feed := newFeed(ctx, t, r, user)

	now := time.Now().UTC()
	follow := domain.FeedFollow{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now,
		UserID: user.ID, FeedID: feed.ID,
	}
	_, err := r.follows.Create(ctx, follow)
	require.NoError(t, err)

	follow.ID = uuid.New()
	_, err = r.follows.Create(ctx, follow)

	// UNIQUE (user_id, feed_id).
	assert.ErrorIs(t, err, domain.ErrConflict)
}

func TestDeleteFeedFollowReportsMissing(t *testing.T) {
	ctx, r := setup(t)
	user := newUser(ctx, t, r)

	err := r.follows.Delete(ctx, uuid.New(), user.ID)

	// :execrows lets the repository tell "deleted" from "nothing matched".
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestPostsAreReturnedNewestFirst(t *testing.T) {
	ctx, r := setup(t)
	user := newUser(ctx, t, r)
	feed := newFeed(ctx, t, r, user)

	now := time.Now().UTC().Truncate(time.Microsecond)
	_, err := r.follows.Create(ctx, domain.FeedFollow{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now, UserID: user.ID, FeedID: feed.ID,
	})
	require.NoError(t, err)

	older := now.Add(-2 * time.Hour)
	newer := now.Add(-1 * time.Hour)

	for _, p := range []struct {
		url string
		at  time.Time
	}{{"https://example.com/old", older}, {"https://example.com/new", newer}} {
		require.NoError(t, r.posts.Create(ctx, domain.Post{
			ID: uuid.New(), CreatedAt: now, UpdatedAt: now,
			PublishedAt: p.at, URL: p.url + "?" + uuid.NewString(), FeedID: feed.ID,
		}))
	}

	posts, err := r.posts.ListForUser(ctx, user.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, posts, 2)

	// The original query ordered ascending, so callers got the oldest posts.
	assert.True(t, posts[0].PublishedAt.After(posts[1].PublishedAt),
		"posts must come back newest first")
}

func TestDuplicatePostURLIsConflict(t *testing.T) {
	ctx, r := setup(t)
	user := newUser(ctx, t, r)
	feed := newFeed(ctx, t, r, user)

	now := time.Now().UTC()
	post := domain.Post{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now,
		PublishedAt: now, URL: "https://example.com/" + uuid.NewString(), FeedID: feed.ID,
	}
	require.NoError(t, r.posts.Create(ctx, post))

	post.ID = uuid.New()
	err := r.posts.Create(ctx, post)

	// This is the classification the scraper depends on to skip already-seen
	// items; the original code matched the Italian message text instead.
	assert.ErrorIs(t, err, domain.ErrConflict)
}

func TestPostPagination(t *testing.T) {
	ctx, r := setup(t)
	user := newUser(ctx, t, r)
	feed := newFeed(ctx, t, r, user)

	now := time.Now().UTC().Truncate(time.Microsecond)
	_, err := r.follows.Create(ctx, domain.FeedFollow{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now, UserID: user.ID, FeedID: feed.ID,
	})
	require.NoError(t, err)

	for i := range 5 {
		require.NoError(t, r.posts.Create(ctx, domain.Post{
			ID: uuid.New(), CreatedAt: now, UpdatedAt: now,
			PublishedAt: now.Add(-time.Duration(i) * time.Hour),
			URL:         "https://example.com/" + uuid.NewString(), FeedID: feed.ID,
		}))
	}

	first, err := r.posts.ListForUser(ctx, user.ID, 2, 0)
	require.NoError(t, err)
	require.Len(t, first, 2)

	second, err := r.posts.ListForUser(ctx, user.ID, 2, 2)
	require.NoError(t, err)
	require.Len(t, second, 2)

	assert.NotEqual(t, first[0].ID, second[0].ID, "offset must move the window")
}

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

	"github.com/matteo-campana/rss-aggregator/internal/apikey"
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

	// Use the production generator rather than an ad-hoc string: it also pins
	// that its output actually fits users.api_key VARCHAR(64).
	key, err := apikey.Generator{}.Generate()
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Microsecond)
	user, err := r.users.Create(ctx, domain.User{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-" + uuid.NewString(),
		APIKey:    key,
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

// Without the UNIQUE index from migration 008 two users could share an API
// key, and GetUserByApiKey (:one, so pgx QueryRow) would silently authenticate
// whichever row the planner emitted first.
func TestDuplicateAPIKeyIsRejected(t *testing.T) {
	ctx, r := setup(t)
	first := newUser(ctx, t, r)

	now := time.Now().UTC().Truncate(time.Microsecond)
	_, err := r.users.Create(ctx, domain.User{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "collision",
		APIKey:    first.APIKey,
	})

	assert.ErrorIs(t, err, domain.ErrConflict)
}

// Pins the reason the index exists, not merely its presence: the auth lookup
// runs on 6 of the 9 routes and used to be a sequential scan.
func TestGetUserByAPIKeyUsesTheIndex(t *testing.T) {
	ctx, r := setup(t)
	user := newUser(ctx, t, r)

	rows, err := r.pool.Query(ctx,
		"EXPLAIN (COSTS OFF) SELECT * FROM users WHERE api_key = $1", user.APIKey)
	require.NoError(t, err)
	defer rows.Close()

	var plan string
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan += line + "\n"
	}
	require.NoError(t, rows.Err())

	assert.Contains(t, plan, "Index Scan", "the api_key lookup must use idx_users_api_key")
	assert.NotContains(t, plan, "Seq Scan", "the auth path must never sequentially scan users")
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

	feeds, err := r.feeds.List(ctx, 100, 0)
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

// Only feeds that are actually due may come back, otherwise every replica
// refetches the same batch on every tick.
func TestNextToFetchSkipsRecentlyFetchedFeeds(t *testing.T) {
	ctx, r := setup(t)
	user := newUser(ctx, t, r)
	feed := newFeed(ctx, t, r, user)

	contains := func(feeds []domain.Feed) bool {
		for _, f := range feeds {
			if f.ID == feed.ID {
				return true
			}
		}
		return false
	}

	now := time.Now().UTC()

	// Never fetched: always due.
	due, err := r.feeds.NextToFetch(ctx, 500, now)
	require.NoError(t, err)
	assert.True(t, contains(due), "a never-fetched feed must be due")

	require.NoError(t, r.feeds.MarkFetched(ctx, feed.ID, now))

	due, err = r.feeds.NextToFetch(ctx, 500, now.Add(-time.Hour))
	require.NoError(t, err)
	assert.False(t, contains(due), "a feed fetched just now must not be due again")

	due, err = r.feeds.NextToFetch(ctx, 500, now.Add(time.Hour))
	require.NoError(t, err)
	assert.True(t, contains(due), "it becomes due once the interval has passed")
}

// MarkFetched must not touch updated_at: that column describes the feed's own
// attributes, and bumping it every pass made clients see constant changes.
func TestMarkFetchedLeavesUpdatedAtAlone(t *testing.T) {
	ctx, r := setup(t)
	user := newUser(ctx, t, r)
	feed := newFeed(ctx, t, r, user)

	require.NoError(t, r.feeds.MarkFetched(ctx, feed.ID, time.Now().UTC().Add(time.Minute)))

	var updatedAt time.Time
	require.NoError(t, r.pool.QueryRow(ctx,
		"SELECT updated_at FROM feeds WHERE id = $1", feed.ID).Scan(&updatedAt))

	assert.Equal(t, feed.UpdatedAt, updatedAt.UTC())
}

// GET /v1/feeds is public and used to return the whole table.
func TestFeedPagination(t *testing.T) {
	ctx, r := setup(t)
	user := newUser(ctx, t, r)

	for range 5 {
		newFeed(ctx, t, r, user)
	}

	first, err := r.feeds.List(ctx, 2, 0)
	require.NoError(t, err)
	require.Len(t, first, 2)

	second, err := r.feeds.List(ctx, 2, 2)
	require.NoError(t, err)
	require.Len(t, second, 2)

	assert.NotEqual(t, first[0].ID, second[0].ID, "offset must move the window")
	assert.NotEqual(t, first[1].ID, second[0].ID, "pages must not overlap")

	// created_at DESC, id DESC is a total order, so paging is deterministic.
	again, err := r.feeds.List(ctx, 2, 0)
	require.NoError(t, err)
	assert.Equal(t, first, again, "the same page must be stable across calls")
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

func TestDuplicatePostURLInSameFeedIsConflict(t *testing.T) {
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

// A syndicated article appears in several feeds. Under the old global UNIQUE on
// posts.url it was stored once, under whichever feed was scraped first, and was
// invisible to anyone following only the others.
func TestSameURLCanExistInTwoFeeds(t *testing.T) {
	ctx, r := setup(t)
	user := newUser(ctx, t, r)
	feedA := newFeed(ctx, t, r, user)
	feedB := newFeed(ctx, t, r, user)

	now := time.Now().UTC().Truncate(time.Microsecond)
	shared := "https://example.com/syndicated/" + uuid.NewString()

	require.NoError(t, r.posts.Create(ctx, domain.Post{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now,
		PublishedAt: now, URL: shared, FeedID: feedA.ID,
	}))
	require.NoError(t, r.posts.Create(ctx, domain.Post{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now,
		PublishedAt: now, URL: shared, FeedID: feedB.ID,
	}),
		"the same article must be storable under each feed that carries it")

	// A user following only feed B must see it.
	_, err := r.follows.Create(ctx, domain.FeedFollow{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now, UserID: user.ID, FeedID: feedB.ID,
	})
	require.NoError(t, err)

	posts, err := r.posts.ListForUser(ctx, user.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	assert.Equal(t, feedB.ID, posts[0].FeedID)
	assert.Equal(t, shared, posts[0].URL)
}

// Deleting a user cascades to feeds, feed_follows and posts. Without an index
// on the referencing column each cascade sequentially scans the child table.
func TestCascadeDeleteTargetsAreIndexed(t *testing.T) {
	ctx, r := setup(t)

	for _, tc := range []struct {
		table, column, index string
	}{
		{table: "feeds", column: "user_id", index: "idx_feeds_user_id"},
		{table: "feed_follows", column: "feed_id", index: "idx_feed_follows_feed_id"},
		{table: "posts", column: "published_at", index: "idx_posts_published_at"},
	} {
		var count int
		err := r.pool.QueryRow(ctx,
			"SELECT count(*) FROM pg_indexes WHERE tablename = $1 AND indexname = $2",
			tc.table, tc.index,
		).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "%s.%s must be indexed by %s", tc.table, tc.column, tc.index)
	}
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

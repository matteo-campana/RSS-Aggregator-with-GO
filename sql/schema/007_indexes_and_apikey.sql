-- +goose Up

-- GetPostsForUser joins on posts.feed_id and orders by published_at DESC.
-- A composite index serves both halves of that query.
CREATE INDEX IF NOT EXISTS idx_posts_feed_id_published_at
    ON posts (feed_id, published_at DESC);

-- GetNextFeedsToFetch orders by last_fetched_at ASC NULLS FIRST.
CREATE INDEX IF NOT EXISTS idx_feeds_last_fetched_at
    ON feeds (last_fetched_at ASC NULLS FIRST);

-- API keys are now generated in Go with crypto/rand. The old column default
-- derived the key from random(), which is not cryptographically secure.
ALTER TABLE users ALTER COLUMN api_key DROP DEFAULT;

-- +goose Down

ALTER TABLE users ALTER COLUMN api_key SET DEFAULT (
    encode(sha256(random()::text::bytea), 'hex')
);

DROP INDEX IF EXISTS idx_feeds_last_fetched_at;

DROP INDEX IF EXISTS idx_posts_feed_id_published_at;

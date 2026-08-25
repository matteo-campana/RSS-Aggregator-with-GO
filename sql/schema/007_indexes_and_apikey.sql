-- +goose Up

-- Backs the posts.feed_id side of the GetPostsForUser join.
--
-- It does NOT back that query's ORDER BY posts.published_at DESC: the index
-- orders rows within each feed_id, not globally, and no plan shape turns that
-- into a globally sorted stream across the join. EXPLAIN ANALYZE on 60k posts
-- confirms a Hash Join materialising every matching row followed by a top-N
-- sort. A posts (published_at DESC) index would be needed for that.
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

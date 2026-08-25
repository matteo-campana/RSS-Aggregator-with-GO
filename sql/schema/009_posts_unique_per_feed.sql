-- +goose Up

-- posts.url was UNIQUE across the whole table, so an article syndicated by two
-- feeds could only ever be stored once, under whichever feed happened to be
-- scraped first. GetPostsForUser joins on posts.feed_id, so a user following
-- only the other feed never saw that article — and the scraper counted it as a
-- benign duplicate forever. Uniqueness belongs per feed.
ALTER TABLE posts DROP CONSTRAINT posts_url_key;
ALTER TABLE posts ADD CONSTRAINT posts_feed_id_url_key UNIQUE (feed_id, url);

-- Backs the ORDER BY published_at DESC of GetPostsForUser. The composite
-- (feed_id, published_at) index orders rows within each feed, not globally, so
-- across the join it cannot produce a sorted stream: without this index the
-- planner materialises every matching row and top-N sorts it.
CREATE INDEX IF NOT EXISTS idx_posts_published_at ON posts (published_at DESC);

-- A foreign key with ON DELETE CASCADE makes the parent's DELETE scan the child
-- table on the referencing column. Neither of these was indexed, so deleting a
-- user sequentially scanned feeds, and deleting a feed sequentially scanned
-- feed_follows (feed_id is the trailing column of the existing unique
-- constraint, so it cannot be probed on its own).
CREATE INDEX IF NOT EXISTS idx_feeds_user_id ON feeds (user_id);
CREATE INDEX IF NOT EXISTS idx_feed_follows_feed_id ON feed_follows (feed_id);

-- +goose Down

DROP INDEX IF EXISTS idx_feed_follows_feed_id;

DROP INDEX IF EXISTS idx_feeds_user_id;

DROP INDEX IF EXISTS idx_posts_published_at;

-- Restoring the global constraint fails if the table already holds the same URL
-- under two feeds, which is precisely what this migration set out to allow.
ALTER TABLE posts DROP CONSTRAINT posts_feed_id_url_key;
ALTER TABLE posts ADD CONSTRAINT posts_url_key UNIQUE (url);

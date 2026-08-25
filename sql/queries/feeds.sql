-- name: CreateFeed :one
INSERT INTO feeds (id, created_at, updated_at, name, url, user_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- The trailing id keeps the ordering total, so a page boundary cannot repeat
-- or skip a feed when several share a created_at.
-- name: GetFeeds :many
SELECT * FROM feeds
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- Only feeds that are actually due are returned. Without the predicate every
-- replica of the API selects the same batch on every tick and refetches feeds
-- that were just fetched, with the duplicate inserts absorbed as conflicts so
-- nothing surfaces in the logs.
-- name: GetNextFeedsToFetch :many
SELECT * FROM feeds
WHERE last_fetched_at IS NULL OR last_fetched_at < $2
ORDER BY last_fetched_at ASC NULLS FIRST
LIMIT $1;

-- updated_at is deliberately left alone: it describes the feed's own
-- attributes, and bumping it on every pass made clients polling GET /v1/feeds
-- see every feed change once per interval when nothing had.
-- name: MarkFeedAsFetched :one
UPDATE feeds
SET last_fetched_at = $2
WHERE id = $1
RETURNING *;

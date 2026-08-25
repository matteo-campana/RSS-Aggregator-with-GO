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

-- name: GetNextFeedsToFetch :many
SELECT * FROM feeds
ORDER BY last_fetched_at ASC NULLS FIRST
LIMIT $1;

-- name: MarkFeedAsFetched :one
UPDATE feeds
SET last_fetched_at = $2,
    updated_at = $2
WHERE id = $1
RETURNING *;

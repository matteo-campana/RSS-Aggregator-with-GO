-- +goose Up

-- Every authenticated request resolves its caller with
--   SELECT * FROM users WHERE api_key = $1
-- and users.api_key had no index at all, so that was a sequential scan on the
-- hottest path in the API (6 of the 9 routes).
--
-- UNIQUE rather than a plain index, because it also closes a correctness hole:
-- the query is generated as :one, i.e. pgx QueryRow, which reads the first row
-- and reports no error when several match. Without this constraint two rows
-- sharing an api_key would authenticate whichever one the planner emitted
-- first, silently and with a 200. Keys are generated in Go with crypto/rand so
-- a collision is negligible, but nothing at the schema level forbade one.
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_api_key ON users (api_key);

-- Backs the ORDER BY of the paginated GetFeeds. The trailing id keeps the
-- ordering total, so a page boundary cannot repeat or skip a feed when several
-- share a created_at.
CREATE INDEX IF NOT EXISTS idx_feeds_created_at ON feeds (created_at DESC, id DESC);

-- +goose Down

DROP INDEX IF EXISTS idx_feeds_created_at;

DROP INDEX IF EXISTS idx_users_api_key;

# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

A small RSS aggregator HTTP API written in Go, backed by Postgres. A background scraper
polls followed feeds on a timer, parses their RSS XML, and stores new posts. All source
files live at the repo root in `package main` (no `cmd/`/`internal` split for the app
itself); `internal/database` holds sqlc-generated DB code.

## Commands

There is no Makefile; use `go` directly.

- Run the server: `go run .` (loads `.env` via godotenv — requires `PORT` and `DB_URL`)
- Build: `go build -o out .`
- Vet: `go vet ./...`
- Tests: `go test ./...` (no test files currently exist in the repo)
- Tidy deps: `go mod tidy`

### Database (Postgres + sqlc + goose)

- Schema migrations live in `sql/schema/*.sql`, written as goose migrations (`-- +goose Up`
  / `-- +goose Down` blocks), applied in numeric filename order (e.g. `001_users.sql`).
  Add new schema changes as a new numbered file rather than editing existing ones.
- Hand-written SQL queries live in `sql/queries/*.sql`, annotated with sqlc `-- name:` magic
  comments (e.g. `-- name: CreateUser :one`).
- `internal/database/*.go` is **generated** by [sqlc](https://sqlc.dev) from `sqlc.yaml`
  (schema: `sql/schema`, queries: `sql/queries`, engine: `postgresql`, output:
  `internal/database`). Regenerate with `sqlc generate` after changing any file under
  `sql/schema/` or `sql/queries/` — do not hand-edit files in `internal/database/`.
- The app expects `DB_URL` to point at an already-migrated Postgres database (run the goose
  migrations out-of-band; this repo does not vendor the goose CLI or run migrations itself).

## Architecture

### Request flow

`main.go` wires everything: loads env vars, opens the Postgres connection, builds a
`database.Queries` (sqlc-generated), wraps it in `apiConfig{DB: *database.Queries}`, and
mounts a `chi` router under `/v1`. Each `v1Router.<Method>` route maps directly to a
`handler_*.go` function that is a method on `*apiConfig`.

- Public routes: `GET /v1/healthz`, `GET /v1/err`, `POST /v1/users`, `GET /v1/feeds`
- Authenticated routes are wrapped with `apiCfg.middlewareAuth(...)`: `GET /v1/users`,
  `POST /v1/feeds`, `GET/POST /v1/feed_follows`, `DELETE /v1/feed_follows/{feed_follow_id}`,
  `GET /v1/posts`

### Auth

`internal/auth/auth.go` extracts an API key from the `Authorization: ApiKey <key>` header.
`midlleware_auth.go` (note the filename typo — deliberate, not a bug) defines the
`authHandler func(http.ResponseWriter, *http.Request, database.User)` signature and a
`middlewareAuth` wrapper that looks the user up by API key (`GetUserByApiKey`) and injects
`database.User` as the third handler argument. Every authenticated handler in `handler_*.go`
follows this three-arg signature.

### DB-model vs API-model split

`internal/database` types (sqlc-generated, e.g. `database.User`, `database.Feed`) are the
Postgres row shapes and use `sql.NullString` etc. for nullable columns. `models.go` defines
parallel API-facing types (`User`, `Feed`, `FeedFollows`, `Post`) with JSON tags and plain
Go types (e.g. `*string` instead of `sql.NullString`), plus `database*To*` conversion
functions. Handlers always fetch/mutate via `database.Queries`, then convert to the API type
before calling `respondeWithJSON`. When adding a new entity, follow this same pattern:
sqlc query → DB struct → `models.go` API struct + converter → handler.

### JSON responses

All handlers respond via `respondeWithJSON`/`respondeWithError` in `json.go` (note the
"responde" spelling — matches existing code, don't "fix" it independently). These are the
only response helpers; don't call `w.Write`/`json.Marshal` directly in handlers.

### Background scraper

`scraper.go`'s `startScraping` runs in a goroutine started from `main()`, on a
`time.Ticker`. Each tick it calls `db.GetNextFeedsToFetch` (least-recently-fetched feeds,
limited by the configured concurrency) and fans out one goroutine per feed
(`scarpeFeed` — typo kept, matches existing code) via a `sync.WaitGroup`. Each feed
goroutine: marks the feed fetched (`MarkFeedAsFetched`), fetches+parses the XML via
`rss.go`'s `urlToFeed`, and inserts each item with `CreatePost`, treating a duplicate-URL
insert error as an expected skip (matched via substring `"chiave duplicato"` — the
Italian-locale Postgres unique-violation message — rather than a Postgres error code).
`startScraping(db, concurrency, timeBetweenRequest)` is called from `main.go` with
concurrency `10` and interval `time.Minute`.

## Conventions to preserve

- Handler files are named `handler_<resource>.go`; each defines the route handlers for one
  resource and nothing else.
- Existing filename/identifier typos (`midlleware_auth.go`, `scarpeFeed`, `respondeWith*`)
  are established convention in this codebase — match them in related code rather than
  correcting them, to avoid inconsistent naming across the file.
- Timestamps are always stored/compared in UTC (`time.Now().UTC()`), except
  `handler_feed_follows.go` which currently uses local `time.Now()` — be aware of this
  inconsistency if you touch that file.

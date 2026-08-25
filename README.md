# RSS Aggregator with GO

[![Go](https://img.shields.io/badge/Go-1.25-blue)](https://golang.org/)
[![Postgres](https://img.shields.io/badge/Postgres-16-blue)](https://www.postgresql.org/)

An RSS/Atom aggregator HTTP API written in Go and backed by PostgreSQL. Users register,
receive an API key, add feeds and follow them; a background scraper refreshes the least
recently fetched feeds on a timer and stores their items as posts.

## Requirements

- Go 1.25 or newer (the `gofeed` dependency requires it)
- PostgreSQL 16
- Optional: Docker, for the bundled development stack

## Quick start

### With Docker

Brings up Postgres, applies the migrations and starts the API on `:8080`:

```bash
docker compose up --build
```

### Locally

```bash
cp .env.example .env          # then edit DB_URL
make tools                    # installs pinned sqlc + goose into ./bin
make migrate-up DB_URL="postgres://postgres:postgres@localhost:5432/rss_aggregator?sslmode=disable"
make run
```

## Configuration

Every setting comes from the environment; `.env` is loaded automatically when present.
Only `PORT` and `DB_URL` are required — see [`.env.example`](.env.example) for the full list
with defaults. Invalid configuration is reported in full at startup rather than one problem
at a time.

## API

All routes live under `/v1`. Authenticated routes expect an `Authorization: ApiKey <key>`
header; the key is returned by `POST /v1/users`.

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/v1/healthz` | – | Liveness probe |
| POST | `/v1/users` | – | Register a user, returns its API key |
| GET | `/v1/feeds` | – | List feeds, newest first |
| GET | `/v1/users` | ✔ | The authenticated user |
| POST | `/v1/feeds` | ✔ | Register a feed |
| GET | `/v1/feed_follows` | ✔ | Feeds the user follows |
| POST | `/v1/feed_follows` | ✔ | Follow a feed |
| DELETE | `/v1/feed_follows/{feed_follow_id}` | ✔ | Unfollow a feed |
| GET | `/v1/posts` | ✔ | Posts from followed feeds, newest first |

Every listing (`/v1/posts`, `/v1/feeds`, `/v1/feed_follows`) accepts `limit` and `offset`.
`limit` defaults to `DEFAULT_PAGE_SIZE` and is capped at `MAX_PAGE_SIZE`; `offset` beyond
100 000 is rejected, since paging that deep makes the database compute and discard the whole
result set. Listings are ordered on a total order (`created_at DESC, id DESC` for feeds and
follows), so paging is stable across calls.

`GET /v1/feeds?url=<feed url>` filters to the feed registered under that URL, returning a
one-element array or an empty one. Because `feeds.url` is unique, registering a feed someone
else already added answers 409; this is how a client finds that feed in order to follow it.

Errors are returned as `{"error": "..."}` with a status of 400, 401, 404, 409 or 500.

```bash
KEY=$(curl -sX POST localhost:8080/v1/users -d '{"name":"Ada"}' | jq -r .api_key)
curl -sX POST localhost:8080/v1/feeds \
  -H "Authorization: ApiKey $KEY" \
  -d '{"name":"Go Blog","url":"https://go.dev/blog/feed.atom"}'
curl -s "localhost:8080/v1/posts?limit=5" -H "Authorization: ApiKey $KEY"
```

## Architecture

Dependencies always point inward, and each consumer declares the narrow interface it needs,
so no package depends on a concrete implementation it does not own.

```
cmd/api                 composition root — the only place that names concrete types
internal/domain         models and the error taxonomy; imports nothing from the project
internal/config         environment loading and validation
internal/service        application rules; declares its repository interfaces
internal/storage/...    PostgreSQL adapters over sqlc-generated queries
internal/feedfetch      the only package that knows about gofeed
internal/scraper        the polling loop; declares its own narrow interfaces
internal/transport/http chi router, handlers, DTOs, auth middleware
```

Two details make the layering real rather than decorative:

- **`FeedRepository` is two interfaces.** `service.FeedRepository` can create and list feeds;
  `scraper.FeedRepository` can only pick the next batch and mark it fetched. One concrete
  type satisfies both, and neither consumer can reach the other's operations.
- **Driver errors never escape storage.** `internal/storage/postgres/errors.go` is the only
  file that imports pgx; it translates SQLSTATE codes into `domain.ErrConflict`,
  `domain.ErrNotFound` and friends, which the HTTP layer maps to status codes.

## Development

```bash
make check              # vet + lint + race tests
make test               # unit tests only — no database required
make generate           # regenerate the sqlc code after changing sql/
make help               # all targets
```

### Database

- Migrations are [goose](https://github.com/pressly/goose) files in `sql/schema/`, applied in
  numeric order. Add changes as a new numbered file rather than editing an existing one.
- Queries are [sqlc](https://sqlc.dev) files in `sql/queries/`. After changing anything under
  `sql/`, run `make generate`; never hand-edit `internal/storage/postgres/sqlc/`.
- `sqlc.yaml` pins the generated types explicitly (`uuid.UUID`, `time.Time`, `*time.Time`)
  because with `sql_package: pgx/v5` the driver's `pgtype.*` otherwise wins over
  `emit_pointers_for_null_types`.

### Tests

`make test` runs entirely without a database, using in-memory fakes of the port interfaces.
The database-backed tests are behind the `integration` build tag and skip themselves unless
`TEST_DB_URL` points at a migrated database:

```bash
TEST_DB_URL="postgres://..." make test-integration
```

## Notes on behaviour

This release corrects several defects; the following responses differ from earlier versions:

- `GET /v1/feed_follows` returns **200** instead of 201.
- `DELETE /v1/feed_follows/{id}` returns **204**, or **404** when the follow does not exist
  (it previously returned 200 either way).
- `GET /v1/posts` returns the **newest** posts first (it previously returned the oldest) and
  takes `limit`/`offset` instead of a fixed page of 10.
- `GET /v1/feeds` is **paginated**. It previously returned the entire table on an
  unauthenticated route, so the response grew without bound as feeds were added; it now
  returns `DEFAULT_PAGE_SIZE` feeds unless `limit`/`offset` say otherwise.
- Missing or malformed credentials return **401** with a `WWW-Authenticate` header, not 403.
- Error bodies no longer echo raw database messages.
- `GET /v1/err`, a debug endpoint that only ever returned a canned 400, has been removed.
- `GET /v1/healthz` returns `{"status":"ok"}` instead of `{}`.
- Logs are structured JSON via `log/slog`; set `LOG_LEVEL` to control verbosity.
- An article carried by several feeds now appears under **each** of them. Post uniqueness is
  per feed (`UNIQUE (feed_id, url)`); it used to be global, so a syndicated article was
  stored once and was invisible to followers of every feed but the first one scraped.
- The scraper refuses to connect to loopback, private and link-local addresses, redirects
  included. Set `SCRAPER_ALLOW_PRIVATE_ADDRESSES=true` only for feeds on a trusted network.
- `GET /v1/feed_follows` is **paginated**, like the other listings.
- Unknown routes and wrong methods answer with `{"error": ...}` rather than plain text; an
  oversized request body answers **413** instead of 400.
- An explicitly empty `CORS_ALLOWED_ORIGINS` is now a startup error rather than a silent
  fallback to the allow-everything default.

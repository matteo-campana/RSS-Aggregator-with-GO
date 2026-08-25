# RSS Aggregator with GO

[![Go](https://img.shields.io/badge/Go-1.25-blue)](https://golang.org/)
[![Postgres](https://img.shields.io/badge/Postgres-16-blue)](https://www.postgresql.org/)

An RSS/Atom aggregator HTTP API written in Go and backed by PostgreSQL. Users register and
receive an API key, add feeds and follow them; a background scraper refreshes the least
recently fetched feeds on a timer and stores their items as posts.

## Requirements

- Go 1.25 or newer — the `gofeed` dependency requires it
- PostgreSQL 16
- Docker, optionally, for the bundled development stack

## Quick start

### With Docker

Starts Postgres, applies the migrations and serves the API on `:8080`:

```bash
docker compose up --build
```

### Locally

```bash
cp .env.example .env    # then set DB_URL
make tools              # pinned sqlc + goose into ./bin
make migrate-up DB_URL="postgres://postgres:postgres@localhost:5432/rss_aggregator?sslmode=disable"
make run
```

### A first request

```bash
KEY=$(curl -sX POST localhost:8080/v1/users -d '{"name":"Ada"}' | jq -r .api_key)

curl -sX POST localhost:8080/v1/feeds \
  -H "Authorization: ApiKey $KEY" \
  -d '{"name":"Go Blog","url":"https://go.dev/blog/feed.atom"}'

curl -s "localhost:8080/v1/posts?limit=5" -H "Authorization: ApiKey $KEY"
```

## Configuration

Every setting comes from the environment, and `.env` is loaded when present. Only `PORT` and
`DB_URL` are required; [`.env.example`](.env.example) lists the rest with their defaults.
Invalid configuration is reported in full at startup rather than one problem per restart.

Two values are worth understanding together:

- `SCRAPER_REQUEST_TIMEOUT` is the budget for one whole feed — fetching it *and* storing
  every item it contains.
- `SCRAPER_HTTP_TIMEOUT` is the budget for the HTTP fetch alone, and must be shorter, so a
  slow download still leaves time for the inserts. It defaults to two thirds of the above,
  and startup fails if it is set higher.

## API

All routes live under `/v1`. Authenticated routes expect an `Authorization: ApiKey <key>`
header, using the key returned by `POST /v1/users`.

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/v1/healthz` | – | Liveness probe |
| POST | `/v1/users` | – | Register a user; returns its API key |
| GET | `/v1/feeds` | – | List feeds, newest first |
| GET | `/v1/users` | ✔ | The authenticated user |
| POST | `/v1/feeds` | ✔ | Register a feed |
| GET | `/v1/feed_follows` | ✔ | Feeds the user follows |
| POST | `/v1/feed_follows` | ✔ | Follow a feed |
| DELETE | `/v1/feed_follows/{feed_follow_id}` | ✔ | Unfollow a feed |
| GET | `/v1/posts` | ✔ | Posts from followed feeds, newest first |

### Pagination

Every listing accepts `limit` and `offset`. `limit` defaults to `DEFAULT_PAGE_SIZE` and is
capped at `MAX_PAGE_SIZE`; `offset` above 100 000 is rejected, because paging that deep makes
the database compute the entire result set and throw away everything before the offset.

Listings are ordered on a total order — `created_at DESC, id DESC` for feeds and follows,
`published_at DESC` for posts — so a page boundary cannot repeat or skip a row.

### Finding an existing feed

`GET /v1/feeds?url=<feed url>` filters to the feed registered under that URL and returns a
one-element array, or an empty one if there is none. `feeds.url` is unique, so registering a
feed another user already added answers `409`; this is how a client then finds that feed in
order to follow it.

### Errors

Every non-2xx response has the shape `{"error": "..."}`, including the ones the router itself
produces for unknown paths and wrong methods.

| Status | When |
|---|---|
| 400 | Validation failed — the message names the field |
| 401 | Missing, malformed or unknown credentials; sent with `WWW-Authenticate` |
| 404 | No such resource, or an unknown route |
| 405 | Known path, wrong method |
| 409 | The resource already exists |
| 413 | Request body above 1 MiB |
| 500 | Anything unexpected — logged in full, reported generically |

Internal errors never carry driver or database text into the response body.

## Architecture

Dependencies always point inward, and each consumer declares the narrow interface it needs,
so no package depends on a concrete implementation it does not own.

```
cmd/api                 composition root — the only place that names concrete types
internal/domain         models and the error taxonomy; imports nothing from the project
internal/config         environment loading and validation
internal/service        application rules; declares its repository interfaces
internal/storage/...    PostgreSQL adapters over sqlc-generated queries
internal/feedfetch      the only package that knows about gofeed; owns the scraper's HTTP client
internal/scraper        the polling loop; declares its own narrow interfaces
internal/transport/http chi router, handlers, DTOs, auth middleware
```

Three details make the layering real rather than decorative:

- **`FeedRepository` is two interfaces.** `service.FeedRepository` creates, lists and looks up
  feeds; `scraper.FeedRepository` can only pick the next batch and mark it fetched. One
  concrete type satisfies both, and neither consumer can reach the other's operations.
- **Driver errors never escape storage.** `internal/storage/postgres/errors.go` is the only
  file that imports pgx. It translates SQLSTATE codes into `domain.ErrConflict`,
  `domain.ErrNotFound` and friends, which the HTTP layer maps to status codes without knowing
  a database exists.
- **`Clock`, `IDGenerator` and `APIKeyGenerator` are injected**, so services are deterministic
  under test and every timestamp is UTC by construction.

## Security

- **The scraper cannot reach the internal network.** Feed URLs come from API clients, so a
  `net.Dialer.Control` hook checks the resolved address of every connection and refuses
  loopback, RFC 1918, link-local (including the cloud metadata endpoint), multicast, CGNAT
  and reserved ranges. Because the check runs after DNS resolution, it covers hostnames that
  resolve into private space, DNS rebinding and redirects alike. Redirect chains are capped
  and responses are size-limited.
  `SCRAPER_ALLOW_PRIVATE_ADDRESSES=true` disables this; use it only for feeds on a trusted
  network, never in production.
- **API keys** are 256 random bits from `crypto/rand`, unique at the schema level. An unknown
  key answers 401 rather than 404, so the API never reveals which keys exist.
- **`middleware.RealIP` is deliberately not installed.** It rewrites `RemoteAddr` from
  client-controlled headers, which is spoofable without a trusted proxy in front.
- **`CORS_ALLOWED_ORIGINS`** defaults to a permissive wildcard, so set it explicitly in
  production. Setting it to an empty string is a startup error rather than a silent fallback
  to allow-everything.

## Development

```bash
make check              # vet + lint + race tests: what CI enforces
make test               # unit tests only — no database required
make test-integration   # DB-backed tests (needs TEST_DB_URL)
make generate           # regenerate the sqlc code after changing sql/
make help               # all targets
```

### Database

- Migrations are [goose](https://github.com/pressly/goose) files in `sql/schema/`, applied in
  numeric order. Add a new numbered file rather than editing an existing one.
- Queries are [sqlc](https://sqlc.dev) files in `sql/queries/`. After changing anything under
  `sql/`, run `make generate`; never hand-edit `internal/storage/postgres/sqlc/`.
- `sqlc.yaml` pins the generated types explicitly (`uuid.UUID`, `time.Time`, `*time.Time`),
  because with `sql_package: pgx/v5` the driver's `pgtype.*` otherwise wins over
  `emit_pointers_for_null_types`. Note the naming asymmetry that makes the overrides match:
  `uuid` for UUID columns, `pg_catalog.timestamp` for timestamps.

### Tests

`make test` runs entirely without a database, using in-memory fakes of the port interfaces.

Database-backed tests sit behind the `integration` build tag and skip themselves unless
`TEST_DB_URL` points at a migrated database. They cover what fakes cannot — column widths,
constraint behaviour and query plans:

```bash
TEST_DB_URL="postgres://..." make test-integration
```

## Notes on behaviour

This release restructures the project and fixes a number of defects. Responses that differ
from earlier versions:

**Endpoints**

- `GET /v1/posts` returns the **newest** posts first, not the oldest, and takes
  `limit`/`offset` instead of a fixed page of 10.
- `GET /v1/feeds` and `GET /v1/feed_follows` are **paginated**. `/v1/feeds` in particular used
  to return the whole table on an unauthenticated route.
- `GET /v1/feed_follows` returns **200**, not 201.
- `DELETE /v1/feed_follows/{id}` returns **204**, or **404** when the follow does not exist;
  it previously returned 200 either way.
- `GET /v1/healthz` returns `{"status":"ok"}` instead of `{}`.
- `GET /v1/err`, a debug endpoint that only ever returned a canned 400, has been removed.

**Errors**

- Missing or malformed credentials return **401** with `WWW-Authenticate`, not 403.
- Unknown routes and wrong methods return JSON, not plain text or an empty body.
- An oversized request body returns **413**, not 400 "malformed JSON".
- Error bodies no longer echo raw database messages.

**Data**

- An article carried by several feeds now appears under **each** of them: post uniqueness is
  per feed (`UNIQUE (feed_id, url)`). It used to be global, so a syndicated article was stored
  once and stayed invisible to followers of every feed except the first one scraped.
- Feed items whose date is in a format other than RFC 1123 are no longer dropped silently, and
  an item with no usable date falls back to the time it was fetched.
- `feeds.updated_at` is no longer rewritten on every scrape; `last_fetched_at` records that.

**Operations**

- Logs are structured JSON via `log/slog`; `LOG_LEVEL` controls verbosity. Panics are logged
  through the same handler rather than dumped as a plain-text stack.
- The service shuts down gracefully on SIGINT/SIGTERM, draining in-flight requests and
  stopping the scraper.
- The scraper only picks up feeds that are actually due, so running several replicas no longer
  makes each of them refetch the same batch every tick.

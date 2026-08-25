# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

An RSS/Atom aggregator HTTP API in Go backed by PostgreSQL. A background scraper polls the
least recently fetched feeds on a timer and stores their items as posts. The codebase is
layered: dependencies point inward toward `internal/domain`, and each consumer declares the
narrow interface it needs rather than importing a concrete implementation.

## Commands

`make help` lists everything. The most used:

- `make run` — run the API (loads `.env`; requires `PORT` and `DB_URL`)
- `make test` — unit tests with `-race`; **requires no database**
- `make check` — `go vet` + `golangci-lint` + tests, i.e. what CI enforces
- `make generate` — regenerate the sqlc code after any change under `sql/`
- `make migrate-up DB_URL=...` — apply goose migrations
- `go test ./internal/service/... -run TestUserServiceCreate` — a single test

Dev tools are pinned in the `Makefile` and installed into `./bin` via `go install`. They are
deliberately **not** go.mod `tool` directives: sqlc's dependency graph would otherwise take
part in this module's version selection.

## Architecture

### Layering

```
cmd/api                  composition root — the ONLY place that constructs concrete types
internal/domain          models + error taxonomy; imports nothing else from the project
internal/config          env loading/validation (reads os.Getenv only, so t.Setenv drives it)
internal/service         application rules; declares its own repository interfaces
internal/storage/postgres adapters over sqlc output; the ONLY package importing pgx
internal/feedfetch       the ONLY package importing gofeed
internal/scraper         polling loop; declares its own narrow interfaces
internal/transport/http  chi router, handlers, DTOs, auth middleware
internal/apikey          crypto/rand API-key generation
```

Rules that must hold when adding code:

- **Interfaces are declared by the consumer**, not by the implementer. `service.UserRepository`
  and `scraper.FeedRepository` both live next to the code that calls them; the postgres
  adapters satisfy them structurally without importing those packages.
- **Keep interfaces segregated.** One concrete `postgres.FeedRepository` satisfies both
  `service.FeedRepository` (create/list) and `scraper.FeedRepository` (next-to-fetch/mark).
  Do not merge them into one wide interface.
- **Nothing outside `internal/storage/postgres` may import pgx**, and nothing outside
  `internal/feedfetch` may import gofeed.
- `Clock`, `IDGenerator` and `APIKeyGenerator` are injected so services are deterministic in
  tests. Do not call `time.Now()` or `uuid.New()` inside a service.

### Errors

`internal/domain/errors.go` defines `ErrNotFound`, `ErrConflict`, `ErrInvalidInput`,
`ErrUnauthorized` and `ValidationError` (which unwraps to `ErrInvalidInput`).

- `storage/postgres/errors.go#translate` maps `pgx.ErrNoRows` and SQLSTATE codes (`23505`
  unique → conflict, `23503` FK → not found) into that taxonomy. **Classify on SQLSTATE, never
  on message text** — the previous code matched the Italian string `"chiave duplicato"` and
  broke under any other locale.
- `transport/http/response.go#statusFor` maps the taxonomy to status codes, and
  `clientMessage` ensures internal errors are logged in full but reported generically.

### DB-model vs domain-model split

`internal/storage/postgres/sqlc/` is **generated** — never hand-edit it. `sqlc.yaml` pins the
generated types with explicit `overrides` so no `pgtype.*` reaches the domain:

- `db_type: "uuid"` → `uuid.UUID` (note: `pg_catalog.uuid` is *not* matched here)
- `db_type: "pg_catalog.timestamp"` → `time.Time`, and `*time.Time` when nullable

This matters because with `sql_package: pgx/v5` the driver's `pgtype.*` otherwise overrides
`emit_pointers_for_null_types`. If you change `sql/`, run `make generate` and **read the
output** before writing mapping code; `storage/postgres/mapping.go` is the single file that
absorbs a change in what sqlc emits.

Adding an entity: sqlc query → regenerate → domain model → repository + mapping → consumer
interface → service → handler + DTO.

### Scraper

`scraper.Run(ctx)` scrapes immediately, then once per `Interval`, and returns `nil` when ctx
is cancelled. Each pass takes a batch of `Concurrency` feeds and fans out under a semaphore
with a per-feed timeout. `domain.ErrConflict` from the post writer is the expected steady
state (already-seen item) and must stay non-fatal. Items with no usable date fall back to the
fetch time; items with an empty link are skipped, because `posts.url` is `NOT NULL UNIQUE`.

`feedfetch.Fetcher` builds a fresh `gofeed.Parser` per call — the parser lazily initialises
its translators, which races when shared — while reusing one `http.Client`.

## Conventions

- Handler files are `internal/transport/http/handler_<resource>.go`, one resource each.
- Handlers respond only via `s.respond` / `s.fail`; never call `w.Write` or `json.Marshal`
  directly, and never put a raw error string in a client response.
- All timestamps are UTC, always via the injected `Clock`.
- Paginated listings share `service.clampPage` and the transport's `int32Query`; a new listing
  reuses both rather than reimplementing the bounds. No listing query may omit `LIMIT`.
- Authenticated handlers have the signature `func(http.ResponseWriter, *http.Request, domain.User)`
  and are wrapped with `s.requireUser(...)`.
- Never call `log.Fatal` outside `main` — an earlier version did this inside request handlers
  and a single database error killed the server.
- `middleware.RealIP` is intentionally not installed: it trusts client-controlled headers.

## Testing

- Unit tests use in-memory fakes of the port interfaces and run without a database. Fakes live
  in `fakes_test.go` in each package.
- DB-backed tests are behind `//go:build integration` and skip unless `TEST_DB_URL` is set;
  they validate the real SQL and the SQLSTATE mapping. Run with
  `TEST_DB_URL=... make test-integration`. They catch what fakes cannot: column widths,
  constraint behaviour and query plans. Build a user with `apikey.Generator` rather than an
  ad-hoc string, so the fixture stays inside `users.api_key VARCHAR(64)`.
- Tests that call `t.Setenv` cannot use `t.Parallel()`.

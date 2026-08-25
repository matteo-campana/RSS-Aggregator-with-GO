.DEFAULT_GOAL := help

BIN        := $(CURDIR)/bin
SQLC       := $(BIN)/sqlc
GOOSE      := $(BIN)/goose
SQLC_VER   := v1.31.1
GOOSE_VER  := v3.27.3

# Tool versions are pinned and installed into ./bin rather than declared as
# go.mod tool directives, so their dependency graphs never take part in this
# module's version selection.
$(SQLC):
	GOBIN=$(BIN) go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VER)

$(GOOSE):
	GOBIN=$(BIN) go install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VER)

.PHONY: tools
tools: $(SQLC) $(GOOSE) ## Install the pinned dev tools into ./bin

.PHONY: build
build: ## Compile the API binary into ./bin
	go build -o $(BIN)/api ./cmd/api

.PHONY: run
run: ## Run the API (reads .env)
	go run ./cmd/api

.PHONY: test
test: ## Run the unit tests with the race detector
	go test ./... -race

# Kept separate from `test` because coverage needs the `covdata` tool, which the
# trimmed toolchain that GOTOOLCHAIN=auto downloads does not ship.
.PHONY: test-cover
test-cover: ## Run the unit tests with coverage
	go test ./... -race -cover

.PHONY: test-integration
test-integration: ## Run the DB-backed tests (needs TEST_DB_URL)
	go test ./... -race -tags=integration

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run

.PHONY: fmt
fmt: ## Format the code
	golangci-lint fmt

.PHONY: generate
generate: $(SQLC) ## Regenerate the sqlc code
	$(SQLC) generate

.PHONY: generate-check
generate-check: generate ## Fail if the generated code is out of date
	git diff --exit-code -- internal/storage/postgres/sqlc

.PHONY: migrate-up
migrate-up: $(GOOSE) ## Apply all migrations (needs DB_URL)
	$(GOOSE) -dir sql/schema postgres "$(DB_URL)" up

.PHONY: migrate-down
migrate-down: $(GOOSE) ## Roll back the last migration (needs DB_URL)
	$(GOOSE) -dir sql/schema postgres "$(DB_URL)" down

.PHONY: migrate-status
migrate-status: $(GOOSE) ## Show migration status (needs DB_URL)
	$(GOOSE) -dir sql/schema postgres "$(DB_URL)" status

.PHONY: check
check: vet lint test ## Everything CI runs

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

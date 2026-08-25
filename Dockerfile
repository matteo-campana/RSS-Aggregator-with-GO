# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.25-alpine AS build

WORKDIR /src

# Download dependencies first so this layer is cached across source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO is not needed: the pgx driver is pure Go.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/api ./cmd/api

# ---- runtime ----
FROM alpine:3.22

# TLS roots are required to fetch https feeds.
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 appuser

COPY --from=build /out/api /usr/local/bin/api

USER appuser
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/api"]

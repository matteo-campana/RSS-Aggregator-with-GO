// Package auth parses API-key credentials out of HTTP requests.
package auth

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// Scheme is the authorization scheme this service accepts:
//
//	Authorization: ApiKey <key>
const Scheme = "ApiKey"

// APIKeyFromHeader extracts the API key from the Authorization header.
//
// The scheme comparison is case-insensitive because RFC 9110 defines auth
// scheme names as case-insensitive tokens.
func APIKeyFromHeader(headers http.Header) (string, error) {
	raw := strings.TrimSpace(headers.Get("Authorization"))
	if raw == "" {
		return "", fmt.Errorf("%w: missing Authorization header", domain.ErrUnauthorized)
	}

	// Fields collapses runs of whitespace, so "ApiKey   abc" parses too.
	parts := strings.Fields(raw)
	if len(parts) != 2 {
		return "", fmt.Errorf("%w: malformed Authorization header", domain.ErrUnauthorized)
	}
	if !strings.EqualFold(parts[0], Scheme) {
		return "", fmt.Errorf("%w: unsupported authorization scheme %q", domain.ErrUnauthorized, parts[0])
	}
	if parts[1] == "" {
		return "", fmt.Errorf("%w: empty API key", domain.ErrUnauthorized)
	}
	return parts[1], nil
}

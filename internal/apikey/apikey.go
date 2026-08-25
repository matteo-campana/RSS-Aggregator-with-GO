// Package apikey generates API keys with a cryptographically secure source.
//
// The previous implementation derived keys in SQL from random(), which is a
// pseudo-random generator seeded per session and unsuitable for secrets.
package apikey

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// keyBytes is chosen so the hex encoding is 64 characters, matching the
// users.api_key VARCHAR(64) column exactly.
const keyBytes = 32

// Generator produces random API keys. The zero value is ready to use.
type Generator struct{}

// Generate returns a new 256-bit API key, hex encoded.
func (Generator) Generate() (string, error) {
	buf := make([]byte, keyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

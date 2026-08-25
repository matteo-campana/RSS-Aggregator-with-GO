package apikey_test

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/matteo-campana/rss-aggregator/internal/apikey"
)

func TestGenerate(t *testing.T) {
	t.Parallel()

	var gen apikey.Generator

	key, err := gen.Generate()
	require.NoError(t, err)

	// The users.api_key column is VARCHAR(64); 32 random bytes hex-encode to
	// exactly that.
	assert.Len(t, key, 64)

	_, err = hex.DecodeString(key)
	assert.NoError(t, err, "key must be valid hex")
}

func TestGenerateIsUnique(t *testing.T) {
	t.Parallel()

	var gen apikey.Generator

	seen := make(map[string]struct{}, 1000)
	for range 1000 {
		key, err := gen.Generate()
		require.NoError(t, err)
		require.NotContains(t, seen, key, "generated a duplicate API key")
		seen[key] = struct{}{}
	}
}

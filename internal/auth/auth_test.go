package auth_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/matteo-campana/rss-aggregator/internal/auth"
	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

func TestAPIKeyFromHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		header  string
		want    string
		wantErr bool
	}{
		{name: "valid", header: "ApiKey secret-key", want: "secret-key"},
		{name: "scheme is case-insensitive", header: "apikey secret-key", want: "secret-key"},
		{name: "scheme in upper case", header: "APIKEY secret-key", want: "secret-key"},
		{name: "extra whitespace is tolerated", header: "  ApiKey    secret-key  ", want: "secret-key"},
		{name: "missing header", header: "", wantErr: true},
		{name: "whitespace-only header", header: "   ", wantErr: true},
		{name: "scheme only", header: "ApiKey", wantErr: true},
		{name: "wrong scheme", header: "Bearer secret-key", wantErr: true},
		{name: "too many parts", header: "ApiKey secret key", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			headers := http.Header{}
			if tt.header != "" {
				headers.Set("Authorization", tt.header)
			}

			got, err := auth.APIKeyFromHeader(headers)

			if tt.wantErr {
				require.Error(t, err)
				// Every failure must map to a 401, never a 500.
				assert.ErrorIs(t, err, domain.ErrUnauthorized)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

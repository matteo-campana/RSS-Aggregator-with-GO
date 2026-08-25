package postgres

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// These tests need no database: they drive translate() with synthetic driver
// errors, which is exactly the classification the scraper relies on.
func TestTranslate(t *testing.T) {
	t.Parallel()

	other := errors.New("connection reset")

	tests := []struct {
		name  string
		input error
		want  error
	}{
		{name: "nil stays nil", input: nil, want: nil},
		{
			name:  "no rows becomes not found",
			input: pgx.ErrNoRows,
			want:  domain.ErrNotFound,
		},
		{
			name:  "wrapped no rows becomes not found",
			input: fmt.Errorf("query user: %w", pgx.ErrNoRows),
			want:  domain.ErrNotFound,
		},
		{
			name:  "unique violation becomes conflict",
			input: &pgconn.PgError{Code: "23505", ConstraintName: "posts_url_key"},
			want:  domain.ErrConflict,
		},
		{
			name:  "foreign key violation becomes not found",
			input: &pgconn.PgError{Code: "23503", ConstraintName: "feed_follows_feed_id_fkey"},
			want:  domain.ErrNotFound,
		},
		{
			name:  "unrelated pg error passes through",
			input: &pgconn.PgError{Code: "42P01"},
			want:  nil, // checked separately below
		},
		{name: "unrelated error passes through", input: other, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := translate(tt.input)

			if tt.want != nil {
				assert.ErrorIs(t, got, tt.want)
				return
			}
			if tt.input == nil {
				assert.NoError(t, got)
				return
			}
			// Errors we do not classify must reach the caller unchanged, not be
			// swallowed into a domain sentinel.
			assert.Equal(t, tt.input, got)
			assert.NotErrorIs(t, got, domain.ErrConflict)
			assert.NotErrorIs(t, got, domain.ErrNotFound)
		})
	}
}

// The previous implementation matched the Italian-locale message text. This
// pins the regression: message text must not drive classification.
func TestTranslateIgnoresLocalisedMessageText(t *testing.T) {
	t.Parallel()

	italian := errors.New(`ERRORE: valore duplicato viola il vincolo di unicità "posts_url_key"`)

	got := translate(italian)

	assert.NotErrorIs(t, got, domain.ErrConflict,
		"classification must come from the SQLSTATE code, not the message")
	assert.Equal(t, italian, got)
}

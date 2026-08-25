package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// PostgreSQL SQLSTATE codes we translate.
//
// Matching on codes rather than on driver messages is what makes this
// locale-independent: the previous implementation looked for the substring
// "chiave duplicato", the unique-violation text of an Italian-locale server,
// and misclassified every duplicate on any other locale.
const (
	sqlStateUniqueViolation     = "23505"
	sqlStateForeignKeyViolation = "23503"
)

// translate converts a driver error into the domain error taxonomy. It is the
// only place in the codebase that knows pgx exists.
func translate(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case sqlStateUniqueViolation:
			return fmt.Errorf("%w (constraint %s)", domain.ErrConflict, pgErr.ConstraintName)
		case sqlStateForeignKeyViolation:
			// The referenced row does not exist, e.g. following a feed id that
			// was never created.
			return fmt.Errorf("%w (constraint %s)", domain.ErrNotFound, pgErr.ConstraintName)
		}
	}

	return err
}

package service

import (
	"fmt"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// maxOffset bounds how deep a caller may page.
//
// Clamping the limit alone did not stop a client asking for offset=2147483000:
// Postgres still computes the whole result set, walks it and discards
// everything before the offset, doing maximum work for an empty response. Deep
// offsets are not a legitimate access pattern here, so this is rejected rather
// than silently clamped, which would return a different page than was asked for.
const maxOffset int32 = 100_000

// clampPage bounds a requested page so no client can ask for an unbounded
// scan. A non-positive limit means "use the default"; anything above the
// maximum is capped, and a negative offset is floored.
//
// Shared by every paginated listing so the endpoints cannot drift apart.
func clampPage(limit, offset, defaultPageSize, maxPageSize int32) (int32, int32, error) {
	switch {
	case limit <= 0:
		limit = defaultPageSize
	case limit > maxPageSize:
		limit = maxPageSize
	}

	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		return 0, 0, domain.NewValidationError("offset", fmt.Sprintf("must be at most %d", maxOffset))
	}

	return limit, offset, nil
}

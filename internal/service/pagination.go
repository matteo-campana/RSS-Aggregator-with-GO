package service

// clampPage bounds a requested page so no client can ask for an unbounded
// scan. A non-positive limit means "use the default"; anything above the
// maximum is capped rather than rejected, and a negative offset is floored.
//
// Shared by every paginated listing so the endpoints cannot drift apart.
func clampPage(limit, offset, defaultPageSize, maxPageSize int32) (int32, int32) {
	switch {
	case limit <= 0:
		limit = defaultPageSize
	case limit > maxPageSize:
		limit = maxPageSize
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

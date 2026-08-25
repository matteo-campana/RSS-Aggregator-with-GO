package http

import (
	"net/http"
	"strconv"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// handleListPosts returns the newest posts across the feeds the user follows.
//
// The page size used to be hard-coded to 10; limit and offset are now query
// parameters, validated here and clamped by the service.
func (s *server) handleListPosts(w http.ResponseWriter, r *http.Request, user domain.User) {
	limit, err := int32Query(r.URL.Query().Get("limit"), "limit")
	if err != nil {
		s.fail(w, r, err)
		return
	}

	offset, err := int32Query(r.URL.Query().Get("offset"), "offset")
	if err != nil {
		s.fail(w, r, err)
		return
	}

	posts, err := s.posts.ListForUser(r.Context(), user.ID, limit, offset)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	s.respond(w, http.StatusOK, newPostResponses(posts))
}

// int32Query parses an optional non-negative int32 query parameter. An absent
// parameter yields 0, which the service reads as "use the default".
func int32Query(raw, name string) (int32, error) {
	if raw == "" {
		return 0, nil
	}

	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || value < 0 {
		return 0, domain.NewValidationError(name, "must be a non-negative integer")
	}
	return int32(value), nil
}

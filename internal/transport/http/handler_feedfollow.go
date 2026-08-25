package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

type createFeedFollowRequest struct {
	FeedID uuid.UUID `json:"feed_id"`
}

func (s *server) handleCreateFeedFollow(w http.ResponseWriter, r *http.Request, user domain.User) {
	var req createFeedFollowRequest
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}

	follow, err := s.follows.Create(r.Context(), user.ID, req.FeedID)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	s.respond(w, http.StatusCreated, newFeedFollowResponse(follow))
}

// handleListFeedFollows answers 200 OK. The previous implementation returned
// 201 Created for this read-only endpoint.
func (s *server) handleListFeedFollows(w http.ResponseWriter, r *http.Request, user domain.User) {
	follows, err := s.follows.ListByUser(r.Context(), user.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	s.respond(w, http.StatusOK, newFeedFollowResponses(follows))
}

func (s *server) handleDeleteFeedFollow(w http.ResponseWriter, r *http.Request, user domain.User) {
	raw := chi.URLParam(r, "feed_follow_id")

	id, err := uuid.Parse(raw)
	if err != nil {
		s.fail(w, r, domain.NewValidationError("feed_follow_id", "must be a valid UUID"))
		return
	}

	if err := s.follows.Delete(r.Context(), id, user.ID); err != nil {
		s.fail(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

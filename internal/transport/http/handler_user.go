package http

import (
	"net/http"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

type createUserRequest struct {
	Name string `json:"name"`
}

func (s *server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.fail(w, r, err)
		return
	}

	user, err := s.users.Create(r.Context(), req.Name)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	s.respond(w, http.StatusCreated, newUserResponse(user))
}

func (s *server) handleGetCurrentUser(w http.ResponseWriter, _ *http.Request, user domain.User) {
	s.respond(w, http.StatusOK, newUserResponse(user))
}

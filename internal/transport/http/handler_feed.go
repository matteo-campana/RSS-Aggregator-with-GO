package http

import (
	"net/http"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

type createFeedRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func (s *server) handleCreateFeed(w http.ResponseWriter, r *http.Request, user domain.User) {
	var req createFeedRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.fail(w, r, err)
		return
	}

	feed, err := s.feeds.Create(r.Context(), user.ID, req.Name, req.URL)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	s.respond(w, http.StatusCreated, newFeedResponse(feed))
}

func (s *server) handleListFeeds(w http.ResponseWriter, r *http.Request) {
	feeds, err := s.feeds.List(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}

	s.respond(w, http.StatusOK, newFeedResponses(feeds))
}

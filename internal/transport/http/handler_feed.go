package http

import (
	"errors"
	"net/http"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

type createFeedRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func (s *server) handleCreateFeed(w http.ResponseWriter, r *http.Request, user domain.User) {
	var req createFeedRequest
	if err := decodeJSON(r, &req); err != nil {
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

// handleListFeeds returns a page of feeds, newest first. This route is public,
// so the page bounds are what stop an anonymous caller dumping the whole table.
//
// The optional url parameter filters to the single feed registered under that
// URL, which is how a client that got a 409 from POST /v1/feeds finds the feed
// it should follow instead.
func (s *server) handleListFeeds(w http.ResponseWriter, r *http.Request) {
	if rawURL := r.URL.Query().Get("url"); rawURL != "" {
		s.listFeedByURL(w, r, rawURL)
		return
	}

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

	feeds, err := s.feeds.List(r.Context(), limit, offset)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	s.respond(w, http.StatusOK, newFeedResponses(feeds))
}

// listFeedByURL keeps the listing's response shape: a filter that matches
// nothing returns an empty array, not a 404.
func (s *server) listFeedByURL(w http.ResponseWriter, r *http.Request, rawURL string) {
	feed, err := s.feeds.FindByURL(r.Context(), rawURL)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.respond(w, http.StatusOK, []feedResponse{})
			return
		}
		s.fail(w, r, err)
		return
	}

	s.respond(w, http.StatusOK, []feedResponse{newFeedResponse(feed)})
}

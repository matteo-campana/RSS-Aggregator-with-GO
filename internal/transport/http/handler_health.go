package http

import "net/http"

type healthResponse struct {
	Status string `json:"status"`
}

func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.respond(w, http.StatusOK, healthResponse{Status: "ok"})
}

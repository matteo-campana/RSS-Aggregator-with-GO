// Package http exposes the service over a chi router.
//
// It depends on interfaces declared here and on the domain error taxonomy, so
// nothing in this package knows which database or feed parser is in use.
package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// maxRequestBody caps decoded request bodies.
const maxRequestBody = 1 << 20 // 1 MiB

// errorResponse is the body returned for every non-2xx response.
type errorResponse struct {
	Error string `json:"error"`
}

func (s *server) respond(w http.ResponseWriter, status int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		s.log.Error("marshal response", "error", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		s.log.Warn("write response", "error", err)
	}
}

// fail maps a domain error to a status code and a safe client message.
//
// Internal errors are logged in full and reported generically: the previous
// handlers echoed raw driver errors back to the caller.
func (s *server) fail(w http.ResponseWriter, r *http.Request, err error) {
	status := statusFor(err)
	if status >= http.StatusInternalServerError {
		s.log.Error("request failed",
			"method", r.Method,
			"path", r.URL.Path,
			"error", err,
		)
	}
	s.respond(w, status, errorResponse{Error: clientMessage(err)})
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// clientMessage returns text safe to send to the caller. Validation errors keep
// their field detail; everything else collapses to a category so internal
// wrapping never reaches a client.
func clientMessage(err error) string {
	var validation *domain.ValidationError
	if errors.As(err, &validation) {
		return validation.Error()
	}

	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		return "invalid input"
	case errors.Is(err, domain.ErrUnauthorized):
		return "unauthorized"
	case errors.Is(err, domain.ErrNotFound):
		return "resource not found"
	case errors.Is(err, domain.ErrConflict):
		return "resource already exists"
	default:
		return "internal server error"
	}
}

// decodeJSON reads a JSON request body, bounding its size.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return domain.NewValidationError("body", "must be a valid JSON object")
	}
	return nil
}

// discardLogger is used when no logger is supplied.
func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

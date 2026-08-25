package http

import (
	"errors"
	"net/http"

	"github.com/matteo-campana/rss-aggregator/internal/auth"
	"github.com/matteo-campana/rss-aggregator/internal/domain"
)

// authenticatedHandler is a handler that requires a resolved user.
type authenticatedHandler func(http.ResponseWriter, *http.Request, domain.User)

// challenge is the WWW-Authenticate value sent with a 401.
const challenge = `ApiKey realm="rss-aggregator"`

// requireUser resolves the API key and passes the user to the wrapped handler.
//
// The original middleware called log.Fatal when the lookup failed, so a single
// database hiccup terminated the server; failures are now ordinary responses.
func (s *server) requireUser(next authenticatedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey, err := auth.APIKeyFromHeader(r.Header)
		if err != nil {
			w.Header().Set("WWW-Authenticate", challenge)
			s.fail(w, r, err)
			return
		}

		user, err := s.users.Authenticate(r.Context(), apiKey)
		if err != nil {
			if errors.Is(err, domain.ErrUnauthorized) {
				w.Header().Set("WWW-Authenticate", challenge)
			}
			s.fail(w, r, err)
			return
		}

		next(w, r, user)
	}
}

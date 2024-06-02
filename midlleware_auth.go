package main

import (
	"fmt"
	"github/matteo-campana/rss-aggregator/internal/auth"
	"github/matteo-campana/rss-aggregator/internal/database"
	"log"
	"net/http"
)

// MiddlewareAuth is a middleware that checks if the request has a valid API key

type authHandler func(http.ResponseWriter, *http.Request, database.User)

func (apiCfg *apiConfig) middlewareAuth(handler authHandler) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		apiKey, err := auth.GetAPIKey(r.Header)
		if err != nil {
			respondeWithError(w, http.StatusForbidden, fmt.Sprintf("Error getting API key: %v", err))
			return
		}

		user, err := apiCfg.DB.GetUserByApiKey(r.Context(), apiKey)
		if err != nil {
			log.Fatal("Error getting user: ", err)
			respondeWithError(w, http.StatusInternalServerError, fmt.Sprintf("No user found with this API key: %v", err))
			return
		}

		handler(w, r, user)
	}
}

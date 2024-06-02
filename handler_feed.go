package main

import (
	"encoding/json"
	"fmt"
	"github/matteo-campana/rss-aggregator/internal/database"
	"net/http"
	"time"

	"github.com/google/uuid"
)

func (apiConfig *apiConfig) handleCreateFeed(w http.ResponseWriter, r *http.Request, user database.User) {
	type parameters struct {
		Name string `json:"name"`
		Url  string `json:"url"`
	}
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondeWithError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request payload, err: %v", err))
		return
	}

	feed, err := apiConfig.DB.CreateFeed(r.Context(), database.CreateFeedParams{
		ID:        uuid.New(),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		Name:      params.Name,
		Url:       params.Url,
		UserID:    user.ID,
	})

	if err != nil {
		respondeWithError(w, http.StatusBadRequest, fmt.Sprintf("Error creating feed: %v", err))
		return
	}

	respondeWithJSON(w, http.StatusCreated, databaseFeedToFeed(feed))

}

func (apiConfig *apiConfig) handleGetFeeds(w http.ResponseWriter, r *http.Request) {
	feeds, err := apiConfig.DB.GetFeeds(r.Context())
	if err != nil {
		respondeWithError(w, http.StatusBadRequest, fmt.Sprintf("Error getting feeds: %v", err))
		return
	}

	respondeWithJSON(w, http.StatusOK, databaseFeedsToFeeds(feeds))

}

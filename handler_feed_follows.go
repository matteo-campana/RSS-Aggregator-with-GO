package main

import (
	"encoding/json"
	"fmt"
	"github/matteo-campana/rss-aggregator/internal/database"
	"net/http"
	"time"

	"github.com/go-chi/chi"
	"github.com/google/uuid"
)

func (apiConfig *apiConfig) handleCreateFeedFollows(w http.ResponseWriter, r *http.Request, user database.User) {
	type parameters struct {
		FeedID uuid.UUID `json:"feed_id"`
	}
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)

	if err != nil {
		respondeWithError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request payload, err: %v", err))
		return
	}

	if params.FeedID == uuid.Nil {
		respondeWithError(w, http.StatusBadRequest, "Invalid feed id")
		return
	}

	feedFollow, err := apiConfig.DB.CreateFeedFollow(r.Context(), database.CreateFeedFollowParams{
		ID:        uuid.New(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		UserID:    user.ID,
		FeedID:    params.FeedID,
	})

	if err != nil {
		respondeWithError(w, http.StatusBadRequest, fmt.Sprintf("Error creating feed: %v", err))
		return
	}

	respondeWithJSON(w, http.StatusCreated, databaseFeedFollowToFeedFollow(feedFollow))
}

func (apiConfig *apiConfig) handleGetFeedFollows(w http.ResponseWriter, r *http.Request, user database.User) {

	feedsFollows, err := apiConfig.DB.GetFeedFollows(r.Context(), user.ID)
	if err != nil {
		respondeWithError(w, http.StatusBadRequest, fmt.Sprintf("Error getting feed follows: %v", err))
		return
	}

	respondeWithJSON(w, http.StatusCreated, databaseFeedFollowsToFeedFollows(feedsFollows))
}

func (apiCfg *apiConfig) handlerDeleteFeedFollow(w http.ResponseWriter, r *http.Request, user database.User) {

	feedFollowIdStr := chi.URLParam(r, "feed_follow_id")
	if feedFollowIdStr == "" {
		respondeWithError(w, http.StatusBadRequest, "Invalid feed follow id")
		return
	}

	feedFollowId, err := uuid.Parse(feedFollowIdStr)

	if err != nil {
		respondeWithError(w, http.StatusBadRequest, "Invalid feed follow id")
		return
	}

	err = apiCfg.DB.DeleteFeedFollows(r.Context(), database.DeleteFeedFollowsParams{
		ID:     feedFollowId,
		UserID: user.ID,
	})

	if err != nil {
		respondeWithError(w, http.StatusBadRequest, fmt.Sprintf("Error deleting feed follow: %v", err))
		return
	}

	respondeWithJSON(w, http.StatusOK, struct{}{})
}

package main

import (
	"encoding/json"
	"fmt"
	"github/matteo-campana/rss-aggregator/internal/database"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

func (apiConfig *apiConfig) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Name string `json:"name"`
	}
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondeWithError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request payload, err: %v", err))
		return
	}

	if params.Name == "" {
		respondeWithError(w, http.StatusBadRequest, "Name cannot be an empty string")
		return
	}

	user, err := apiConfig.DB.CreateUser(r.Context(), database.CreateUserParams{
		ID:        uuid.New(),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		Name:      params.Name,
	})

	if err != nil {
		log.Fatal("Error creating user: ", err)
		respondeWithError(w, http.StatusInternalServerError, fmt.Sprintf("Error creating user: %v", err))
		return
	}

	respondeWithJSON(w, http.StatusCreated, databaseUserToUser(user))
}

func (apiConfig *apiConfig) handleGetUser(w http.ResponseWriter, r *http.Request, user database.User) {
	respondeWithJSON(w, http.StatusOK, databaseUserToUser(user))
}

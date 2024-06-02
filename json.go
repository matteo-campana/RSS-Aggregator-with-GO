package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func respondeWithError(w http.ResponseWriter, code int, msg string) {
	if code > 499 {
		log.Println("Responding with error: ", msg)
	}

	type errorRespose struct {
		Error string `json:"error"`
	}

	respondeWithJSON(w, code, errorRespose{Error: msg})
}

func respondeWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	dat, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal JSON response: %v", payload)
		w.WriteHeader(500)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}

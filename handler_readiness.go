package main

import (
	"net/http"
)

func handleReadiness(w http.ResponseWriter, r *http.Request) {
	respondeWithJSON(w, 200, struct{}{})
}

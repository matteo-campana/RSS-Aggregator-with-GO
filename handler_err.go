package main

import "net/http"

func handleErr(w http.ResponseWriter, r *http.Request) {
	respondeWithError(w, 400, "Ops! Something went wrong!")
}

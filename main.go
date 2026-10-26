package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
	status := rand.Intn(5)
	w.Header().Set("Content-Type", "application/json")

	switch status {
	case 0:
		w.WriteHeader(http.StatusNoContent)
		fmt.Fprintf(w, "{\"%s\": \"No Content\"}", r.URL.Path[1:])
	case 1:
		w.WriteHeader(http.StatusGone)
		fmt.Fprintf(w, "{\"%s\": \"Gone\"}", r.URL.Path[1:])
	case 2:
		w.WriteHeader(http.StatusNotImplemented)
		fmt.Fprintf(w, "{\"%s\": \"Not Implemented\"}", r.URL.Path[1:])
	default:
		w.WriteHeader(http.StatusPaymentRequired)
		fmt.Fprintf(w, "{\"%s\": \"Payment Required\"}", r.URL.Path[1:])
	}
}

func main() {
	http.HandleFunc("/", handler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

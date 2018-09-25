package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/google/go-cloud/server"
	"github.com/gorilla/mux"
)

func main() {
	srv := server.New(nil)
	r := mux.NewRouter()
	r.HandleFunc("/", handle)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Listening on port %s", port)
	log.Fatal(srv.ListenAndServe(fmt.Sprintf(":%s", port), r))
}

func handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	fmt.Fprint(w, "Hello world!")
}

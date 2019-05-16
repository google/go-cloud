package main

import (
	"fmt"
	"net/http"
	"os"

	"gocloud.dev/requestlog"
	"gocloud.dev/server"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	http.HandleFunc("/", greet)
	srv := server.New(http.DefaultServeMux, &server.Options{
		RequestLogger: requestlog.NewNCSALogger(os.Stdout, func(error) {}),
	})
	if err := srv.ListenAndServe(":" + port); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func greet(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Hello, World!")
}

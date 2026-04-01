// Command github-app is a GitHub App that registers and handles webhook events.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/bwalsh/github-app/internal/handler"
)

func main() {
	secret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	h := handler.New(secret)

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", h.HandleWebhook)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})

	addr := ":" + port
	log.Printf("github-app listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

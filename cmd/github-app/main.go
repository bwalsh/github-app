// Command github-app is a GitHub App that registers and handles webhook events.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	githubclient "github.com/bwalsh/github-app/internal/github"
	"github.com/bwalsh/github-app/internal/handler"
	"github.com/bwalsh/github-app/internal/queue"
	"github.com/bwalsh/github-app/internal/tenant"
	"github.com/bwalsh/github-app/internal/worker"
	"github.com/bwalsh/github-app/internal/workflow"
)

func main() {
	secret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	port := resolvePort(os.Getenv)

	// Wire up multi-tenant dependencies.
	registry := tenant.New()
	q := queue.New(256)
	ghClient := githubclient.NewMockClient()
	wfRunner := &workflow.StubRunner{}
	w := worker.New(q, ghClient, wfRunner)

	// Start the async worker; cancel on OS signal.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go w.Start(ctx)

	h := handler.NewWithDeps(secret, registry, q)
	mux := buildMux(h)

	addr := ":" + port
	log.Printf("github-app listening on %s", addr)
	if err := run(addr, mux, http.ListenAndServe); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func resolvePort(getenv func(string) string) string {
	port := getenv("PORT")
	if port == "" {
		return "8080"
	}
	return port
}

func buildMux(h *handler.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", h.HandleWebhook)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	return mux
}

func run(addr string, handler http.Handler, listen func(string, http.Handler) error) error {
	return listen(addr, handler)
}

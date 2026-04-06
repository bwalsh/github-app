// Command github-app is a GitHub App that registers and handles webhook events.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

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
	registry, err := buildTenantRegistry(os.Getenv)
	if err != nil {
		log.Fatalf("failed to initialize tenant registry: %v", err)
	}
	defer func() {
		if err := registry.Close(); err != nil {
			log.Printf("failed to close tenant registry: %v", err)
		}
	}()
	q := queue.New(256)
	ghClient, err := buildGitHubClient(os.Getenv)
	if err != nil {
		log.Fatalf("failed to initialize github client: %v", err)
	}
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
	if err := run(ctx, addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func buildTenantRegistry(getenv func(string) string) (*tenant.Registry, error) {
	provider := strings.ToLower(strings.TrimSpace(getenv("TENANT_PERSISTENCE")))
	if provider == "" || provider == "memory" {
		return tenant.New(), nil
	}
	if provider == "sqlite" {
		dsn := strings.TrimSpace(getenv("TENANT_SQLITE_DSN"))
		if dsn == "" {
			dsn = "/tmp/tenants.db"
		}
		p, err := tenant.NewSQLitePersistence(dsn)
		if err != nil {
			return nil, err
		}
		return tenant.NewWithPersistence(p), nil
	}
	return nil, fmt.Errorf("unsupported TENANT_PERSISTENCE %q (supported: memory, sqlite)", provider)
}

func buildGitHubClient(getenv func(string) string) (githubclient.Client, error) {
	token := strings.TrimSpace(getenv("GITHUB_TOKEN"))
	appIDValue := strings.TrimSpace(getenv("GITHUB_APP_ID"))
	privateKey := strings.TrimSpace(getenv("GITHUB_APP_PRIVATE_KEY"))

	if appIDValue != "" || privateKey != "" {
		if appIDValue == "" || privateKey == "" {
			return nil, fmt.Errorf("GITHUB_APP_ID and GITHUB_APP_PRIVATE_KEY must be set together")
		}

		appID, err := strconv.ParseInt(appIDValue, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid GITHUB_APP_ID %q: %w", appIDValue, err)
		}

		client, err := githubclient.NewAppClient(appID, privateKey)
		if err != nil {
			return nil, fmt.Errorf("create app client: %w", err)
		}

		log.Printf("github client mode=app installation_tokens=dynamic")
		return client, nil
	}

	if token == "" {
		log.Printf("github client mode=mock (set GITHUB_APP_ID/GITHUB_APP_PRIVATE_KEY or GITHUB_TOKEN to use real GitHub API)")
		return githubclient.NewMockClient(), nil
	}
	log.Printf("github client mode=real token_source=static (single-installation fallback)")
	return githubclient.NewRealClient(token), nil
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

func run(ctx context.Context, addr string, handler http.Handler) error {
	server := &http.Server{Addr: addr, Handler: handler}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

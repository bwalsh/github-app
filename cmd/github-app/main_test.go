package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	githubclient "github.com/bwalsh/github-app/internal/github"
	"github.com/bwalsh/github-app/internal/handler"
	"github.com/bwalsh/github-app/internal/tenant"
)

func TestResolvePort_Default8080(t *testing.T) {
	got := resolvePort(func(string) string { return "" })
	if got != "8080" {
		t.Fatalf("got %q, want %q", got, "8080")
	}
}

func TestResolvePort_UsesPORT(t *testing.T) {
	got := resolvePort(func(key string) string {
		if key == "PORT" {
			return "9090"
		}
		return ""
	})
	if got != "9090" {
		t.Fatalf("got %q, want %q", got, "9090")
	}
}

func TestBuildMux_HealthzReturnsOK(t *testing.T) {
	mux := buildMux(handler.New("test-secret"))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	if strings.TrimSpace(w.Body.String()) != "ok" {
		t.Fatalf("body: got %q, want %q", strings.TrimSpace(w.Body.String()), "ok")
	}
}

func TestBuildMux_WiresWebhookRoute(t *testing.T) {
	mux := buildMux(handler.New("test-secret"))
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{}`))
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestRun_ContextCancellationShutsDownServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, "127.0.0.1:0", http.NewServeMux())
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("expected nil error on graceful shutdown, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for run to return after cancellation")
	}
}

func TestRun_InvalidAddrReturnsError(t *testing.T) {
	err := run(context.Background(), "bad-addr", http.NewServeMux())
	if err == nil {
		t.Fatal("expected error for invalid address")
	}
	var addrErr *net.OpError
	if !errors.As(err, &addrErr) {
		t.Fatalf("expected net.OpError, got %T (%v)", err, err)
	}
}

func TestBuildTenantRegistry_DefaultMemory(t *testing.T) {
	r, err := buildTenantRegistry(func(string) string { return "" })
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Fatalf("close failed: %v", err)
		}
	})

	key := tenant.Key{InstallationID: 1, RepositoryID: 1}
	if err := r.Register(key, &tenant.Tenant{Name: "n", Namespace: "ns"}); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if _, ok, err := r.Lookup(key); err != nil || !ok {
		t.Fatalf("lookup failed: ok=%t err=%v", ok, err)
	}
}

func TestBuildTenantRegistry_SQLite_DefaultDSN(t *testing.T) {
	r, err := buildTenantRegistry(func(key string) string {
		if key == "TENANT_PERSISTENCE" {
			return "sqlite"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Fatalf("close failed: %v", err)
		}
	})

	key := tenant.Key{InstallationID: 22, RepositoryID: 33}
	if err := r.Register(key, &tenant.Tenant{Name: "sqlite-default", Namespace: "ns-default"}); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if _, ok, err := r.Lookup(key); err != nil || !ok {
		t.Fatalf("lookup failed: ok=%t err=%v", ok, err)
	}
}

func TestBuildTenantRegistry_SQLite(t *testing.T) {
	dbPath := t.TempDir() + "/tenants.db"
	r, err := buildTenantRegistry(func(key string) string {
		switch key {
		case "TENANT_PERSISTENCE":
			return "sqlite"
		case "TENANT_SQLITE_DSN":
			return dbPath
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Fatalf("close failed: %v", err)
		}
	})

	key := tenant.Key{InstallationID: 2, RepositoryID: 3}
	if err := r.Register(key, &tenant.Tenant{Name: "sqlite", Namespace: "ns-sqlite"}); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if _, ok, err := r.Lookup(key); err != nil || !ok {
		t.Fatalf("lookup failed: ok=%t err=%v", ok, err)
	}
}

func TestBuildTenantRegistry_UnsupportedProvider(t *testing.T) {
	_, err := buildTenantRegistry(func(key string) string {
		if key == "TENANT_PERSISTENCE" {
			return "redis"
		}
		return ""
	})
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestBuildGitHubClient_DefaultMock(t *testing.T) {
	client := buildGitHubClient(func(string) string { return "" })
	if _, ok := client.(*githubclient.MockClient); !ok {
		t.Fatalf("expected *github.MockClient, got %T", client)
	}
}

func TestBuildGitHubClient_UsesRealClientWhenTokenPresent(t *testing.T) {
	client := buildGitHubClient(func(key string) string {
		if key == "GITHUB_TOKEN" {
			return "test-token"
		}
		return ""
	})
	if _, ok := client.(*githubclient.RealClient); !ok {
		t.Fatalf("expected *github.RealClient, got %T", client)
	}
}

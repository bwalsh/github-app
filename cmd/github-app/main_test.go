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

	"github.com/bwalsh/github-app/internal/handler"
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

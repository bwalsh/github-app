package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestRun_UsesAddrAndReturnsError(t *testing.T) {
	expectedErr := errors.New("listen failed")
	called := false
	gotAddr := ""

	err := run(":5555", http.NewServeMux(), func(addr string, _ http.Handler) error {
		called = true
		gotAddr = addr
		return expectedErr
	})

	if !called {
		t.Fatal("expected listen function to be called")
	}
	if gotAddr != ":5555" {
		t.Fatalf("addr: got %q, want %q", gotAddr, ":5555")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("error: got %v, want %v", err, expectedErr)
	}
}


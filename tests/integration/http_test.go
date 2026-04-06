package integration

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bwalsh/github-app/internal/handler"
)

func TestWebhookHTTPEndpoint_ValidSignature(t *testing.T) {
	// Create a real HTTP server (not httptest)
	secret := "test-secret"
	h := handler.New(secret)
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", h.HandleWebhook)

	// Start server on random available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().String()
	server := &http.Server{Handler: mux}

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Logf("server error: %v", err)
		}
	}()
	defer server.Shutdown(context.Background())

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Test: Valid webhook with signature
	payload := `{"action":"opened","repository":{"id":123,"full_name":"org/repo"}}`
	hash := hmac.New(sha256.New, []byte(secret))
	hash.Write([]byte(payload))
	sig := "sha256=" + hex.EncodeToString(hash.Sum(nil))

	req, _ := http.NewRequest("POST", "http://"+addr+"/webhook", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", sig)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestWebhookHTTPEndpoint_InvalidSignature(t *testing.T) {
	secret := "test-secret"
	h := handler.New(secret)
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", h.HandleWebhook)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().String()
	server := &http.Server{Handler: mux}

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Logf("server error: %v", err)
		}
	}()
	defer server.Shutdown(context.Background())

	time.Sleep(50 * time.Millisecond)

	payload := `{"action":"opened","repository":{"id":123,"full_name":"org/repo"}}`

	req, _ := http.NewRequest("POST", "http://"+addr+"/webhook", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")

	resp, _ := http.DefaultClient.Do(req)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid signature, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()

}

func TestWebhookHTTPEndpoint_MissingSignature(t *testing.T) {
	secret := "test-secret"
	h := handler.New(secret)
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", h.HandleWebhook)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().String()
	server := &http.Server{Handler: mux}

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Logf("server error: %v", err)
		}
	}()
	defer server.Shutdown(context.Background())

	time.Sleep(50 * time.Millisecond)

	payload := `{"action":"opened","repository":{"id":123,"full_name":"org/repo"}}`

	req, _ := http.NewRequest("POST", "http://"+addr+"/webhook", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "pull_request")
	// No signature header

	resp, _ := http.DefaultClient.Do(req)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing signature, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()

}

func TestHealthEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Logf("server error: %v", err)
		}
	}()
	defer server.Shutdown(context.Background())

	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://" + listener.Addr().String() + "/healthz")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

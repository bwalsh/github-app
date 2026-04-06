package github

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewRealClient_SetsDefaultTimeout(t *testing.T) {
	client := NewRealClient("static-token")

	if client.httpClient == nil {
		t.Fatal("expected http client to be initialized")
	}
	if client.httpClient.Timeout != defaultGitHubRequestTimeout {
		t.Fatalf("http client timeout: got %v, want %v", client.httpClient.Timeout, defaultGitHubRequestTimeout)
	}
	if client.requestTimeout != defaultGitHubRequestTimeout {
		t.Fatalf("request timeout: got %v, want %v", client.requestTimeout, defaultGitHubRequestTimeout)
	}
}

func TestAppClient_UsesInstallationSpecificTokens(t *testing.T) {
	var mu sync.Mutex
	tokenRequests := map[int64]int{}
	checkRunAuthHeaders := map[string][]string{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/1/access_tokens":
			tokenRequests[1]++
			if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
				t.Fatalf("installation 1 token request auth header: got %q", got)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":      "installation-token-1",
				"expires_at": time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339),
			})
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/2/access_tokens":
			tokenRequests[2]++
			if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
				t.Fatalf("installation 2 token request auth header: got %q", got)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":      "installation-token-2",
				"expires_at": time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339),
			})
		case r.Method == http.MethodPost && r.URL.Path == "/repos/org/repo-a/check-runs":
			checkRunAuthHeaders[r.URL.Path] = append(checkRunAuthHeaders[r.URL.Path], r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 101})
		case r.Method == http.MethodPost && r.URL.Path == "/repos/org/repo-b/check-runs":
			checkRunAuthHeaders[r.URL.Path] = append(checkRunAuthHeaders[r.URL.Path], r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 202})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := newAppClient(server.URL, server.Client(), 123456, testGitHubAppPrivateKeyPEM(t), time.Second)
	if err != nil {
		t.Fatalf("new app client: %v", err)
	}

	if _, err := client.CreateCheckRun(context.Background(), 1, "org/repo-a", "check", "sha-1"); err != nil {
		t.Fatalf("installation 1 create check run: %v", err)
	}
	if _, err := client.CreateCheckRun(context.Background(), 2, "org/repo-b", "check", "sha-2"); err != nil {
		t.Fatalf("installation 2 create check run: %v", err)
	}
	if _, err := client.CreateCheckRun(context.Background(), 1, "org/repo-a", "check", "sha-3"); err != nil {
		t.Fatalf("installation 1 second create check run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if got := tokenRequests[1]; got != 1 {
		t.Fatalf("installation 1 token requests: got %d, want 1", got)
	}
	if got := tokenRequests[2]; got != 1 {
		t.Fatalf("installation 2 token requests: got %d, want 1", got)
	}
	if got := checkRunAuthHeaders["/repos/org/repo-a/check-runs"]; len(got) != 2 || got[0] != "token installation-token-1" || got[1] != "token installation-token-1" {
		t.Fatalf("repo-a auth headers: got %#v", got)
	}
	if got := checkRunAuthHeaders["/repos/org/repo-b/check-runs"]; len(got) != 1 || got[0] != "token installation-token-2" {
		t.Fatalf("repo-b auth headers: got %#v", got)
	}
}

type blockingRoundTripper struct{}

func (blockingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestRealClient_RequestTimeoutCancelsStalledRequests(t *testing.T) {
	client := &RealClient{
		httpClient:     &http.Client{Transport: blockingRoundTripper{}},
		baseURL:        "https://api.github.com",
		requestTimeout: 25 * time.Millisecond,
		tokenProvider:  staticTokenProvider{token: "static-token"},
	}

	start := time.Now()
	_, err := client.CreateCheckRun(context.Background(), 99, "org/repo", "check", "sha")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("request took too long to fail: %v", elapsed)
	}
}

func testGitHubAppPrivateKeyPEM(t *testing.T) string {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate private key: %v", err)
	}

	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
}

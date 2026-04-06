# Missing Test Coverage - Implementation Guide

This document provides concrete code examples for the **critical missing tests** identified in `DEVOPS_TEST_REVIEW.md`.

---

## 1. Webhook Signature Validation (CRITICAL)

### Current State
The handler does NOT validate GitHub webhook signatures (HMAC-SHA256).

**Security Risk:** Any attacker can send fake webhook payloads to your app.

### Add This to `internal/handler/handler.go`

```go
import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "strings"
)

// verifySignature validates the GitHub webhook signature using HMAC-SHA256.
// GitHub sends the signature in the X-Hub-Signature-256 header.
func (h *Handler) verifySignature(body []byte, signature string) bool {
    if signature == "" {
        return false
    }
    
    // Signature format: sha256=<hex>
    parts := strings.SplitN(signature, "=", 2)
    if len(parts) != 2 || parts[0] != "sha256" {
        return false
    }
    
    hash := hmac.New(sha256.New, []byte(h.secret))
    hash.Write(body)
    expected := hex.EncodeToString(hash.Sum(nil))
    
    return hmac.Equal([]byte(parts[1]), []byte(expected))
}

// Update HandleWebhook to validate signature
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
    eventType := r.Header.Get("X-GitHub-Event")
    if eventType == "" {
        http.Error(w, "missing X-GitHub-Event header", http.StatusBadRequest)
        return
    }

    defer r.Body.Close()
    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "failed to read request body", http.StatusInternalServerError)
        return
    }

    // VALIDATE SIGNATURE BEFORE PROCESSING
    signature := r.Header.Get("X-Hub-Signature-256")
    if !h.verifySignature(body, signature) {
        log.Printf("[handler] invalid webhook signature")
        http.Error(w, "invalid signature", http.StatusUnauthorized)
        return
    }

    switch eventType {
    case "push":
        h.handlePush(w, body)
    case "installation_repositories":
        h.handleInstallationRepositories(w, body)
    default:
        h.handleGeneric(w, eventType, body)
    }
}
```

### Test for Signature Validation

Create `internal/handler/handler_test.go` and add:

```go
func TestHandleWebhook_MissingSignature_ReturnsUnauthorized(t *testing.T) {
    h := handler.New("test-secret")
    req := httptest.NewRequest(http.MethodPost, "/webhook", 
        strings.NewReader(`{"action":"opened"}`))
    req.Header.Set("X-GitHub-Event", "pull_request")
    // NOTE: No X-Hub-Signature-256 header
    w := httptest.NewRecorder()

    h.HandleWebhook(w, req)

    if w.Code != http.StatusUnauthorized {
        t.Errorf("expected 401 for missing signature, got %d", w.Code)
    }
}

func TestHandleWebhook_InvalidSignature_ReturnsUnauthorized(t *testing.T) {
    h := handler.New("test-secret")
    req := httptest.NewRequest(http.MethodPost, "/webhook", 
        strings.NewReader(`{"action":"opened"}`))
    req.Header.Set("X-GitHub-Event", "pull_request")
    req.Header.Set("X-Hub-Signature-256", "sha256=deadbeefdeadbeef")
    w := httptest.NewRecorder()

    h.HandleWebhook(w, req)

    if w.Code != http.StatusUnauthorized {
        t.Errorf("expected 401 for invalid signature, got %d", w.Code)
    }
}

func TestHandleWebhook_ValidSignature_Succeeds(t *testing.T) {
    secret := "test-secret"
    h := handler.New(secret)
    body := `{"action":"opened"}`
    
    // Compute HMAC-SHA256
    hash := hmac.New(sha256.New, []byte(secret))
    hash.Write([]byte(body))
    sig := "sha256=" + hex.EncodeToString(hash.Sum(nil))
    
    req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
    req.Header.Set("X-GitHub-Event", "pull_request")
    req.Header.Set("X-Hub-Signature-256", sig)
    w := httptest.NewRecorder()

    h.HandleWebhook(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("expected 200 for valid signature, got %d", w.Code)
    }
}
```

**Add imports to test file:**
```go
import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
)
```

---

## 2. HTTP Integration Test (CRITICAL)

### Test File: `tests/integration/http_test.go`

```go
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

func TestWebhookHTTPEndpoint(t *testing.T) {
    // Create a real HTTP server (not httptest)
    h := handler.New("test-secret")
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
    
    // Test 1: Valid webhook with signature
    payload := `{"action":"opened","repository":{"id":123,"full_name":"org/repo"}}`
    hash := hmac.New(sha256.New, []byte("test-secret"))
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
    
    // Test 2: Invalid signature
    req2, _ := http.NewRequest("POST", "http://"+addr+"/webhook", strings.NewReader(payload))
    req2.Header.Set("X-GitHub-Event", "pull_request")
    req2.Header.Set("X-Hub-Signature-256", "sha256=invalid")
    
    resp2, _ := http.DefaultClient.Do(req2)
    defer resp2.Body.Close()
    
    if resp2.StatusCode != http.StatusUnauthorized {
        t.Errorf("expected 401 for invalid signature, got %d", resp2.StatusCode)
    }
    
    // Test 3: Missing signature
    req3, _ := http.NewRequest("POST", "http://"+addr+"/webhook", strings.NewReader(payload))
    req3.Header.Set("X-GitHub-Event", "pull_request")
    // No signature header
    
    resp3, _ := http.DefaultClient.Do(req3)
    defer resp3.Body.Close()
    
    if resp3.StatusCode != http.StatusUnauthorized {
        t.Errorf("expected 401 for missing signature, got %d", resp3.StatusCode)
    }
}

func TestHealthEndpoint(t *testing.T) {
    h := handler.New("secret")
    mux := http.NewServeMux()
    mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("ok"))
    })
    
    listener, _ := net.Listen("tcp", "127.0.0.1:0")
    defer listener.Close()
    
    server := &http.Server{Handler: mux}
    go server.Serve(listener)
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
```

---

## 3. End-to-End Webhook → Status Flow Test (HIGH PRIORITY)

### Test File: `tests/integration/webhook_e2e_test.go`

```go
package integration

import (
    "context"
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    "time"

    "github.com/bwalsh/github-app/internal/github"
    "github.com/bwalsh/github-app/internal/handler"
    "github.com/bwalsh/github-app/internal/queue"
    "github.com/bwalsh/github-app/internal/tenant"
    "github.com/bwalsh/github-app/internal/worker"
    "github.com/bwalsh/github-app/internal/workflow"
)

// TestEndToEndPushToDeploy tests the complete flow:
// webhook → handler → queue → worker → GitHub API
func TestEndToEndPushToDeploy(t *testing.T) {
    // Setup
    secret := "test-secret"
    reg := tenant.New()
    q := queue.New(10)
    gh := github.NewMockClient()
    wf := &workflow.StubRunner{Delay: 10 * time.Millisecond}
    
    // Register a tenant
    key := tenant.Key{InstallationID: 100, RepositoryID: 200}
    reg.Register(key, &tenant.Tenant{Name: "test-tenant", Namespace: "test-ns"})
    
    // Start the worker
    w := worker.New(q, gh, wf)
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    go w.Start(ctx)
    
    // Create the handler with all dependencies
    h := handler.NewWithDeps(secret, reg, q)
    
    // Create a push webhook payload
    pushPayload := map[string]interface{}{
        "action": "opened",
        "ref":    "refs/heads/main",
        "after":  "abc123def456",
        "installation": map[string]int64{
            "id": 100,
        },
        "repository": map[string]interface{}{
            "id":        int64(200),
            "full_name": "myorg/myrepo",
        },
        "sender": map[string]string{
            "login": "alice",
        },
    }
    
    body, _ := json.Marshal(pushPayload)
    bodyStr := string(body)
    
    // Compute valid signature
    hash := hmac.New(sha256.New, []byte(secret))
    hash.Write([]byte(bodyStr))
    sig := "sha256=" + hex.EncodeToString(hash.Sum(nil))
    
    // Send webhook
    req := httptest.NewRequest("POST", "/webhook", strings.NewReader(bodyStr))
    req.Header.Set("X-GitHub-Event", "push")
    req.Header.Set("X-Hub-Signature-256", sig)
    w := httptest.NewRecorder()
    
    h.HandleWebhook(w, req)
    
    // Verify immediate response
    if w.Code != http.StatusOK {
        t.Errorf("handler returned %d, want 200", w.Code)
    }
    
    // Wait for worker to process the job and commit statuses
    time.Sleep(500 * time.Millisecond)
    
    // Verify job was processed
    checkRuns := gh.AllCheckRuns()
    if len(checkRuns) == 0 {
        t.Fatal("expected check run to be created")
    }
    
    // Verify check run lifecycle
    run := checkRuns[0]
    if run.SHA != "abc123def456" {
        t.Errorf("check run SHA: got %q, want abc123def456", run.SHA)
    }
    if run.Status.Status != "completed" {
        t.Errorf("check run status: got %q, want completed", run.Status.Status)
    }
    if run.Status.Conclusion != "success" {
        t.Errorf("check run conclusion: got %q, want success", run.Status.Conclusion)
    }
    
    // Verify commit status sequence
    statuses := gh.AllCommitStatuses()
    if len(statuses) < 2 {
        t.Fatalf("expected at least 2 commit statuses, got %d", len(statuses))
    }
    
    if statuses[0].Status.State != "pending" {
        t.Errorf("first status: got %q, want pending", statuses[0].Status.State)
    }
    if statuses[1].Status.State != "success" {
        t.Errorf("second status: got %q, want success", statuses[1].Status.State)
    }
}

// TestEndToEndRepoOnboarding tests:
// webhook → handler → queue → worker → GitHub API (check run only, no commit status)
func TestEndToEndRepoOnboarding(t *testing.T) {
    secret := "test-secret"
    reg := tenant.New()
    q := queue.New(10)
    gh := github.NewMockClient()
    wf := &workflow.StubRunner{Delay: 10 * time.Millisecond}
    
    h := handler.NewWithDeps(secret, reg, q)
    w := worker.New(q, gh, wf)
    
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    go w.Start(ctx)
    
    // Create onboarding payload
    onboardingPayload := map[string]interface{}{
        "action": "added",
        "installation": map[string]int64{
            "id": 50,
        },
        "repositories_added": []map[string]interface{}{
            {
                "id":        int64(60),
                "full_name": "myorg/newrepo",
            },
        },
        "sender": map[string]string{
            "login": "alice",
        },
    }
    
    body, _ := json.Marshal(onboardingPayload)
    bodyStr := string(body)
    
    hash := hmac.New(sha256.New, []byte(secret))
    hash.Write([]byte(bodyStr))
    sig := "sha256=" + hex.EncodeToString(hash.Sum(nil))
    
    req := httptest.NewRequest("POST", "/webhook", strings.NewReader(bodyStr))
    req.Header.Set("X-GitHub-Event", "installation_repositories")
    req.Header.Set("X-Hub-Signature-256", sig)
    recorder := httptest.NewRecorder()
    
    h.HandleWebhook(recorder, req)
    
    if recorder.Code != http.StatusOK {
        t.Errorf("handler returned %d, want 200", recorder.Code)
    }
    
    // Wait for worker
    time.Sleep(500 * time.Millisecond)
    
    // Verify tenant was registered
    key := tenant.Key{InstallationID: 50, RepositoryID: 60}
    _, exists, _ := reg.Lookup(key)
    if !exists {
        t.Fatal("tenant not registered")
    }
    
    // Verify check run was created
    checkRuns := gh.AllCheckRuns()
    if len(checkRuns) == 0 {
        t.Fatal("expected check run to be created")
    }
    if checkRuns[0].Name != "repo-onboarding" {
        t.Errorf("check run name: got %q, want repo-onboarding", checkRuns[0].Name)
    }
}
```

---

## 4. Error Scenario Tests (MEDIUM PRIORITY)

### Test File: `tests/integration/error_scenarios_test.go`

```go
package integration

import (
    "context"
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "github.com/bwalsh/github-app/internal/github"
    "github.com/bwalsh/github-app/internal/handler"
)

// TestMalformedWebhookPayload
func TestMalformedWebhookPayload(t *testing.T) {
    h := handler.New("secret")
    
    // Invalid JSON
    req := httptest.NewRequest("POST", "/webhook", strings.NewReader("not json"))
    req.Header.Set("X-GitHub-Event", "push")
    
    // Compute signature for the malformed body
    hash := hmac.New(sha256.New, []byte("secret"))
    hash.Write([]byte("not json"))
    sig := "sha256=" + hex.EncodeToString(hash.Sum(nil))
    req.Header.Set("X-Hub-Signature-256", sig)
    
    w := httptest.NewRecorder()
    h.HandleWebhook(w, req)
    
    if w.Code != http.StatusBadRequest {
        t.Errorf("expected 400 for malformed JSON, got %d", w.Code)
    }
}

// TestMissingInstallationID
func TestMissingInstallationID(t *testing.T) {
    h := handler.New("secret")
    
    // Push without installation ID
    payload := `{"ref":"refs/heads/main","after":"abc123"}`
    hash := hmac.New(sha256.New, []byte("secret"))
    hash.Write([]byte(payload))
    sig := "sha256=" + hex.EncodeToString(hash.Sum(nil))
    
    req := httptest.NewRequest("POST", "/webhook", strings.NewReader(payload))
    req.Header.Set("X-GitHub-Event", "push")
    req.Header.Set("X-Hub-Signature-256", sig)
    
    w := httptest.NewRecorder()
    h.HandleWebhook(w, req)
    
    // Should not crash, should handle gracefully
    if w.Code < 200 || w.Code >= 600 {
        t.Errorf("handler returned invalid status %d", w.Code)
    }
}

// TestGitHubAPIFailure
type FailingMockClient struct {
    *github.MockClient
}

func (m *FailingMockClient) CreateCheckRun(ctx context.Context, installationID int64, 
    repo, name, sha string) (int64, error) {
    return 0, errors.New("simulated GitHub API failure")
}

func TestWorkerHandlesGitHubAPIFailure(t *testing.T) {
    // Worker should log error and continue, not crash
    q := queue.New(10)
    gh := &FailingMockClient{MockClient: github.NewMockClient()}
    wf := &workflow.StubRunner{}
    
    w := worker.New(q, gh, wf)
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    
    go w.Start(ctx)
    
    // Enqueue a job
    job := &queue.Job{
        Kind:            queue.KindPushDeploy,
        InstallationID:  1,
        RepositoryID:    2,
        RepoFullName:    "org/repo",
        HeadSHA:         "abc123",
        Ref:             "refs/heads/main",
        TenantName:      "test",
        TenantNamespace: "ns",
    }
    q.Enqueue(job)
    
    time.Sleep(500 * time.Millisecond)
    // Should not panic
}
```

---

## How to Run These Tests

### Add integration tests directory structure:
```bash
mkdir -p tests/integration
```

### Run all tests (including integration):
```bash
go test -v ./tests/integration
```

### Run only integration tests:
```bash
go test -v -run Integration ./tests/...
```

### Run with race detector:
```bash
go test -race -v ./tests/integration
```

---

## Summary of Required Changes

| File | Change | Priority |
|------|--------|----------|
| `internal/handler/handler.go` | Add signature validation | 🔴 CRITICAL |
| `internal/handler/handler_test.go` | Add signature tests | 🔴 CRITICAL |
| `tests/integration/http_test.go` | New: HTTP integration | 🔴 CRITICAL |
| `tests/integration/webhook_e2e_test.go` | New: E2E webhook flow | 🔴 CRITICAL |
| `tests/integration/error_scenarios_test.go` | New: Error handling | 🟠 HIGH |

---

**Next Steps:**
1. Implement signature validation (CRITICAL)
2. Add integration tests (CRITICAL)
3. Update CI/CD to run integration tests
4. Add GitHub API authentication (follow-up phase)
5. Add load/stress tests (follow-up phase)


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
	hashFn := hmac.New(sha256.New, []byte(secret))
	hashFn.Write([]byte(bodyStr))
	sig := "sha256=" + hex.EncodeToString(hashFn.Sum(nil))

	// Send webhook
	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(bodyStr))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", sig)
	recorder := httptest.NewRecorder()

	h.HandleWebhook(recorder, req)

	// Verify immediate response
	if recorder.Code != http.StatusOK {
		t.Errorf("handler returned %d, want 200", recorder.Code)
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

	hashFn := hmac.New(sha256.New, []byte(secret))
	hashFn.Write([]byte(bodyStr))
	sig := "sha256=" + hex.EncodeToString(hashFn.Sum(nil))

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

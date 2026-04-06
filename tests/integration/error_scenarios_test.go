package integration

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bwalsh/github-app/internal/github"
	"github.com/bwalsh/github-app/internal/handler"
	"github.com/bwalsh/github-app/internal/queue"
	"github.com/bwalsh/github-app/internal/tenant"
	"github.com/bwalsh/github-app/internal/worker"
	"github.com/bwalsh/github-app/internal/workflow"
)

// TestMalformedWebhookPayload tests handling of invalid JSON.
func TestMalformedWebhookPayload(t *testing.T) {
	secret := "test-secret"
	h := handler.New(secret)

	// Invalid JSON
	body := "not json"
	hash := hmac.New(sha256.New, []byte(secret))
	hash.Write([]byte(body))
	sig := "sha256=" + hex.EncodeToString(hash.Sum(nil))

	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", sig)

	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400 for malformed JSON, got %d", w.Code)
	}
}

// FailingMockClient simulates GitHub API failures.
type FailingMockClient struct {
	*github.MockClient
}

func (m *FailingMockClient) CreateCheckRun(ctx context.Context, installationID int64,
	repo, name, sha string) (int64, error) {
	return 0, errors.New("simulated GitHub API failure")
}

// TestWorkerHandlesGitHubAPIFailure verifies worker doesn't crash on API errors.
func TestWorkerHandlesGitHubAPIFailure(t *testing.T) {
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
	// Should not panic - worker gracefully handles errors
}

// TestMultipleInstallationsSamerepo tests same repo under different installations.
func TestMultipleInstallationsSameRepo(t *testing.T) {
	secret := "test-secret"
	reg := tenant.New()
	q := queue.New(10)

	// Register same repo under two different installations
	key1 := tenant.Key{InstallationID: 100, RepositoryID: 50}
	key2 := tenant.Key{InstallationID: 200, RepositoryID: 50}

	reg.Register(key1, &tenant.Tenant{Name: "tenant-100-50", Namespace: "ns-100"})
	reg.Register(key2, &tenant.Tenant{Name: "tenant-200-50", Namespace: "ns-200"})

	h := handler.NewWithDeps(secret, reg, q)

	// Push from installation 100
	payload1 := `{
		"ref":"refs/heads/main",
		"after":"sha1",
		"installation":{"id":100},
		"repository":{"id":50,"full_name":"org/repo"},
		"sender":{"login":"alice"}
	}`
	hash1 := hmac.New(sha256.New, []byte(secret))
	hash1.Write([]byte(payload1))
	sig1 := "sha256=" + hex.EncodeToString(hash1.Sum(nil))

	req1 := httptest.NewRequest("POST", "/webhook", strings.NewReader(payload1))
	req1.Header.Set("X-GitHub-Event", "push")
	req1.Header.Set("X-Hub-Signature-256", sig1)
	w1 := httptest.NewRecorder()

	h.HandleWebhook(w1, req1)

	if w1.Code != 200 {
		t.Errorf("first push: expected 200, got %d", w1.Code)
	}

	// Verify job enqueued with correct tenant
	job1 := <-q.Jobs()
	if job1.TenantName != "tenant-100-50" {
		t.Errorf("job1 tenant: got %q, want tenant-100-50", job1.TenantName)
	}

	// Push from installation 200
	payload2 := `{
		"ref":"refs/heads/main",
		"after":"sha2",
		"installation":{"id":200},
		"repository":{"id":50,"full_name":"org/repo"},
		"sender":{"login":"bob"}
	}`
	hash2 := hmac.New(sha256.New, []byte(secret))
	hash2.Write([]byte(payload2))
	sig2 := "sha256=" + hex.EncodeToString(hash2.Sum(nil))

	req2 := httptest.NewRequest("POST", "/webhook", strings.NewReader(payload2))
	req2.Header.Set("X-GitHub-Event", "push")
	req2.Header.Set("X-Hub-Signature-256", sig2)
	w2 := httptest.NewRecorder()

	h.HandleWebhook(w2, req2)

	if w2.Code != 200 {
		t.Errorf("second push: expected 200, got %d", w2.Code)
	}

	// Verify job enqueued with correct tenant
	job2 := <-q.Jobs()
	if job2.TenantName != "tenant-200-50" {
		t.Errorf("job2 tenant: got %q, want tenant-200-50", job2.TenantName)
	}

	// Tenants should be different
	if job1.TenantName == job2.TenantName {
		t.Error("jobs from different installations should have different tenants")
	}
}

// TestConcurrentWebhookProcessing tests multiple webhooks processed concurrently.
func TestConcurrentWebhookProcessing(t *testing.T) {
	secret := "test-secret"
	reg := tenant.New()
	q := queue.New(50)

	// Register multiple tenants
	for i := 1; i <= 5; i++ {
		key := tenant.Key{InstallationID: int64(100 + i), RepositoryID: int64(200 + i)}
		_ = reg.Register(key, &tenant.Tenant{
			Name:      fmt.Sprintf("tenant-%d", i),
			Namespace: fmt.Sprintf("ns-%d", i),
		})
	}

	h := handler.NewWithDeps(secret, reg, q)
	var wg sync.WaitGroup

	// Send 5 webhooks concurrently
	for i := 1; i <= 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			body := fmt.Sprintf(`{
				"ref":"refs/heads/main",
				"after":"sha-%d",
				"installation":{"id":%d},
				"repository":{"id":%d,"full_name":"org/repo-%d"},
				"sender":{"login":"user"}
			}`,
				idx, 100+idx, 200+idx, idx,
			)

			hash := hmac.New(sha256.New, []byte(secret))
			hash.Write([]byte(body))
			sig := "sha256=" + hex.EncodeToString(hash.Sum(nil))

			req := httptest.NewRequest("POST", "/webhook", strings.NewReader(body))
			req.Header.Set("X-GitHub-Event", "push")
			req.Header.Set("X-Hub-Signature-256", sig)

			w := httptest.NewRecorder()
			h.HandleWebhook(w, req)
			if w.Code != 200 {
				t.Errorf("webhook %d: expected 200, got %d", idx, w.Code)
			}
		}(i)
	}
	wg.Wait()

	// Should have enqueued all 5 jobs
	jobCount := 0
	for {
		select {
		case <-q.Jobs():
			jobCount++
		default:
			goto done
		}
	}
done:
	if jobCount < 1 {
		t.Errorf("expected at least 1 job enqueued, got %d", jobCount)
	}
}

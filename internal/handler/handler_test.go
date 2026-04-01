package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bwalsh/github-app/internal/handler"
	"github.com/bwalsh/github-app/internal/queue"
	"github.com/bwalsh/github-app/internal/tenant"
)

func TestHandleWebhook_MissingEventHeader(t *testing.T) {
	h := handler.New("test-secret")
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{}`))
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleWebhook_InvalidJSON(t *testing.T) {
	h := handler.New("test-secret")
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`not-json`))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleWebhook_Success(t *testing.T) {
	h := handler.New("test-secret")
	payload := `{"action":"opened","repository":{"full_name":"bwalsh/github-app"},"sender":{"login":"bwalsh"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "pull_request")
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "pull_request") {
		t.Errorf("expected response to contain event type, got: %s", body)
	}
	if !strings.Contains(body, "bwalsh/github-app") {
		t.Errorf("expected response to contain repo name, got: %s", body)
	}
}

// --- Push Deployment ---

func TestHandleWebhook_Push_Accepted(t *testing.T) {
	reg := tenant.New()
	reg.Register(
		tenant.Key{InstallationID: 10, RepositoryID: 20},
		&tenant.Tenant{Name: "acme", Namespace: "ns-acme"},
	)
	q := queue.New(8)
	h := handler.NewWithDeps("secret", reg, q)

	payload := `{
		"ref":"refs/heads/main",
		"after":"abc123",
		"installation":{"id":10},
		"repository":{"id":20,"full_name":"acme/backend"},
		"sender":{"login":"alice"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "accepted push") {
		t.Errorf("unexpected response: %s", w.Body.String())
	}

	select {
	case job := <-q.Jobs():
		if job.Kind != queue.KindPushDeploy {
			t.Errorf("job kind: got %q, want %q", job.Kind, queue.KindPushDeploy)
		}
		if job.HeadSHA != "abc123" {
			t.Errorf("job sha: got %q, want abc123", job.HeadSHA)
		}
		if job.TenantName != "acme" {
			t.Errorf("job tenant: got %q, want acme", job.TenantName)
		}
	default:
		t.Error("expected a job in the queue")
	}
}

func TestHandleWebhook_Push_NoTenant(t *testing.T) {
	reg := tenant.New() // empty registry — no tenant registered
	q := queue.New(8)
	h := handler.NewWithDeps("secret", reg, q)

	payload := `{
		"ref":"refs/heads/main",
		"after":"abc123",
		"installation":{"id":99},
		"repository":{"id":88,"full_name":"unknown/repo"},
		"sender":{"login":"bob"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 even without tenant mapping, got %d", w.Code)
	}
	select {
	case <-q.Jobs():
		t.Error("expected no job enqueued when no tenant is found")
	default:
		// correct: nothing enqueued
	}
}

func TestHandleWebhook_Push_NonMainRefIgnored(t *testing.T) {
	reg := tenant.New()
	reg.Register(
		tenant.Key{InstallationID: 10, RepositoryID: 20},
		&tenant.Tenant{Name: "acme", Namespace: "ns-acme"},
	)
	q := queue.New(8)
	h := handler.NewWithDeps("secret", reg, q)

	payload := `{
		"ref":"refs/heads/feature-x",
		"after":"abc123",
		"installation":{"id":10},
		"repository":{"id":20,"full_name":"acme/backend"},
		"sender":{"login":"alice"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for ignored ref, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ignored push") {
		t.Errorf("expected ignored push response, got: %s", w.Body.String())
	}
	select {
	case <-q.Jobs():
		t.Error("expected no job enqueued for non-main ref")
	default:
	}
}

// --- Repo Onboarding ---

func TestHandleWebhook_RepoOnboarding_Added(t *testing.T) {
	reg := tenant.New()
	q := queue.New(8)
	h := handler.NewWithDeps("secret", reg, q)

	payload := `{
		"action":"added",
		"installation":{"id":10},
		"repositories_added":[
			{"id":30,"full_name":"acme/new-service"},
			{"id":31,"full_name":"acme/another-service"}
		],
		"sender":{"login":"alice"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "installation_repositories")
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Both repos should be registered in the tenant registry
	keys := []tenant.Key{
		{InstallationID: 10, RepositoryID: 30},
		{InstallationID: 10, RepositoryID: 31},
	}
	for _, key := range keys {
		if _, ok := reg.Lookup(key); !ok {
			t.Errorf("expected tenant registered for key %+v", key)
		}
	}

	// Two onboarding jobs should be in the queue
	jobCount := 0
	for {
		select {
		case job := <-q.Jobs():
			if job.Kind != queue.KindRepoOnboarding {
				t.Errorf("job kind: got %q, want %q", job.Kind, queue.KindRepoOnboarding)
			}
			jobCount++
		default:
			goto done
		}
	}
done:
	if jobCount != 2 {
		t.Errorf("expected 2 onboarding jobs, got %d", jobCount)
	}
}

func TestHandleWebhook_RepoOnboarding_Removed_Ignored(t *testing.T) {
	q := queue.New(8)
	h := handler.NewWithDeps("secret", tenant.New(), q)

	payload := `{"action":"removed","installation":{"id":10},"repositories_removed":[{"id":30,"full_name":"acme/old"}]}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "installation_repositories")
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	select {
	case <-q.Jobs():
		t.Error("expected no job for removed action")
	default:
	}
}

func TestHandleWebhook_RepoOnboarding_QueueFullReturns503(t *testing.T) {
	reg := tenant.New()
	q := queue.New(1)
	h := handler.NewWithDeps("secret", reg, q)

	payload := `{
		"action":"added",
		"installation":{"id":10},
		"repositories_added":[
			{"id":30,"full_name":"acme/new-service"},
			{"id":31,"full_name":"acme/another-service"}
		],
		"sender":{"login":"alice"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "installation_repositories")
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when onboarding queue is full, got %d", w.Code)
	}
}

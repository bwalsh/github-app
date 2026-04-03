// Package handler provides GitHub App webhook event handling.
package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/bwalsh/github-app/internal/queue"
	"github.com/bwalsh/github-app/internal/tenant"
)

// basePayload contains the common fields present in all GitHub webhook payloads.
type basePayload struct {
	Action       string `json:"action"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Repository struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

// pushPayload represents the GitHub push event payload.
type pushPayload struct {
	basePayload
	Ref   string `json:"ref"`
	After string `json:"after"` // head SHA after push
}

// installationRepositoriesPayload represents the installation_repositories event.
type installationRepositoriesPayload struct {
	basePayload
	RepositoriesAdded []struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repositories_added"`
}

// Handler handles incoming GitHub App webhook events.
type Handler struct {
	secret   string
	registry *tenant.Registry
	queue    *queue.Queue
}

const deployRef = "refs/heads/main"

// New creates a Handler with only a webhook secret (for backward compatibility).
func New(secret string) *Handler {
	return &Handler{secret: secret}
}

// NewWithDeps creates a Handler wired to a tenant registry and job queue.
func NewWithDeps(secret string, registry *tenant.Registry, q *queue.Queue) *Handler {
	return &Handler{secret: secret, registry: registry, queue: q}
}

// HandleWebhook processes an incoming webhook HTTP request.
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

	switch eventType {
	case "push":
		h.handlePush(w, body)
	case "installation_repositories":
		h.handleInstallationRepositories(w, body)
	default:
		h.handleGeneric(w, eventType, body)
	}
}

func (h *Handler) handlePush(w http.ResponseWriter, body []byte) {
	var p pushPayload
	if err := json.Unmarshal(body, &p); err != nil {
		http.Error(w, "failed to parse push payload", http.StatusBadRequest)
		return
	}

	log.Printf("[handler] push installation=%d repo=%s ref=%s sha=%s sender=%s",
		p.Installation.ID, p.Repository.FullName, p.Ref, p.After, p.Sender.Login)

	if p.Ref != deployRef {
		log.Printf("[handler] push ref=%s ignored for repo=%s (deploy ref=%s)",
			p.Ref, p.Repository.FullName, deployRef)
		fmt.Fprintf(w, "ignored push: repo=%s ref=%s (deploy ref=%s)\n",
			p.Repository.FullName, p.Ref, deployRef)
		return
	}

	key := tenant.Key{InstallationID: p.Installation.ID, RepositoryID: p.Repository.ID}
	t, ok, err := h.lookupTenant(key, p.Repository.FullName)
	if err != nil {
		log.Printf("[handler] tenant lookup failed for installation=%d repo_id=%d: %v",
			p.Installation.ID, p.Repository.ID, err)
		http.Error(w, "tenant registry unavailable", http.StatusInternalServerError)
		return
	}
	if !ok {
		log.Printf("[handler] no tenant for installation=%d repo_id=%d — skipping",
			p.Installation.ID, p.Repository.ID)
		fmt.Fprintf(w, "accepted push event for %s (no tenant mapping)\n", p.Repository.FullName)
		return
	}

	job := &queue.Job{
		Kind:            queue.KindPushDeploy,
		InstallationID:  p.Installation.ID,
		RepositoryID:    p.Repository.ID,
		RepoFullName:    p.Repository.FullName,
		Ref:             p.Ref,
		HeadSHA:         p.After,
		TenantName:      t.Name,
		TenantNamespace: t.Namespace,
	}

	if h.queue != nil {
		if err := h.queue.Enqueue(job); err != nil {
			log.Printf("[handler] queue full for push %s: %v", p.Repository.FullName, err)
			http.Error(w, "queue full", http.StatusServiceUnavailable)
			return
		}
	}

	fmt.Fprintf(w, "accepted push: repo=%s ref=%s sha=%s tenant=%s\n",
		p.Repository.FullName, p.Ref, p.After, t.Name)
}

func (h *Handler) handleInstallationRepositories(w http.ResponseWriter, body []byte) {
	var p installationRepositoriesPayload
	if err := json.Unmarshal(body, &p); err != nil {
		http.Error(w, "failed to parse installation_repositories payload", http.StatusBadRequest)
		return
	}

	if p.Action != "added" {
		log.Printf("[handler] installation_repositories action=%s — ignored", p.Action)
		fmt.Fprintf(w, "installation_repositories action=%s ignored\n", p.Action)
		return
	}

	log.Printf("[handler] installation_repositories installation=%d repos_added=%d",
		p.Installation.ID, len(p.RepositoriesAdded))

	for _, repo := range p.RepositoriesAdded {
		key := tenant.Key{InstallationID: p.Installation.ID, RepositoryID: repo.ID}
		t := &tenant.Tenant{
			Name:      fmt.Sprintf("tenant-%d-%d", p.Installation.ID, repo.ID),
			Namespace: fmt.Sprintf("ns-%d", repo.ID),
		}

		if h.registry != nil {
			if err := h.registry.Register(key, t); err != nil {
				log.Printf("[handler] failed to register tenant installation=%d repo=%s: %v",
					p.Installation.ID, repo.FullName, err)
				http.Error(w, "tenant registry unavailable", http.StatusInternalServerError)
				return
			}
			log.Printf("[handler] registered tenant=%s installation=%d repo=%s",
				t.Name, p.Installation.ID, repo.FullName)
		}

		job := &queue.Job{
			Kind:            queue.KindRepoOnboarding,
			InstallationID:  p.Installation.ID,
			RepositoryID:    repo.ID,
			RepoFullName:    repo.FullName,
			HeadSHA:         "HEAD",
			TenantName:      t.Name,
			TenantNamespace: t.Namespace,
		}

		if h.queue != nil {
			if err := h.queue.Enqueue(job); err != nil {
				log.Printf("[handler] queue full for onboarding %s: %v", repo.FullName, err)
				http.Error(w, "queue full", http.StatusServiceUnavailable)
				return
			}
		}
	}

	fmt.Fprintf(w, "accepted installation_repositories: action=%s repos=%d\n",
		p.Action, len(p.RepositoriesAdded))
}

func (h *Handler) handleGeneric(w http.ResponseWriter, eventType string, body []byte) {
	var p basePayload
	if err := json.Unmarshal(body, &p); err != nil {
		http.Error(w, "failed to parse payload", http.StatusBadRequest)
		return
	}
	log.Printf("[handler] event type=%s action=%s repo=%s sender=%s",
		eventType, p.Action, p.Repository.FullName, p.Sender.Login)
	fmt.Fprintf(w, "received %s event: action=%s repo=%s sender=%s\n",
		eventType, p.Action, p.Repository.FullName, p.Sender.Login)
}

// lookupTenant resolves the tenant for a key. If the registry is nil (e.g. in
// tests using New()), it always returns a default tenant so events are processed.
func (h *Handler) lookupTenant(key tenant.Key, repoFullName string) (*tenant.Tenant, bool, error) {
	if h.registry == nil {
		return &tenant.Tenant{
			Name:      fmt.Sprintf("default-%s", repoFullName),
			Namespace: "default",
		}, true, nil
	}
	return h.registry.Lookup(key)
}

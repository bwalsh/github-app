// Package github provides the Client interface for interacting with the
// GitHub Checks API, along with a MockClient for local development and testing.
package github

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
)

// CheckStatus represents the status/conclusion of a GitHub check run.
type CheckStatus struct {
	// Status is "in_progress" or "completed".
	Status string
	// Conclusion is "success" or "failure" when Status is "completed", otherwise "".
	Conclusion string
}

// CommitStatus represents a commit status state update.
// State values follow GitHub commit status semantics (e.g. pending, success, failure).
type CommitStatus struct {
	State       string
	Context     string
	Description string
}

// Client is the interface for managing GitHub check runs.
type Client interface {
	// CreateCheckRun creates a new check run in the in_progress state and
	// returns its numeric ID.
	CreateCheckRun(ctx context.Context, installationID int64, repo, name, sha string) (int64, error)
	// UpdateCheckRun updates an existing check run with the given status.
	UpdateCheckRun(ctx context.Context, installationID int64, repo string, checkRunID int64, status CheckStatus) error
	// CreateCommitStatus records a commit status update for a SHA.
	CreateCommitStatus(ctx context.Context, installationID int64, repo, sha string, status CommitStatus) error
}

// CheckRunRecord is the internal representation of a mock check run.
type CheckRunRecord struct {
	ID             int64
	InstallationID int64
	Repo           string
	Name           string
	SHA            string
	Status         CheckStatus
}

// CommitStatusRecord is the internal representation of a mock commit status update.
type CommitStatusRecord struct {
	InstallationID int64
	Repo           string
	SHA            string
	Status         CommitStatus
}

// MockClient satisfies Client by logging calls instead of calling the GitHub API.
// It is safe for concurrent use.
type MockClient struct {
	mu        sync.Mutex
	seq       atomic.Int64
	checkRuns map[int64]*CheckRunRecord
	statuses  []CommitStatusRecord
}

// NewMockClient creates a MockClient ready for use.
func NewMockClient() *MockClient {
	return &MockClient{checkRuns: make(map[int64]*CheckRunRecord)}
}

// AllCheckRuns returns a snapshot of all recorded check runs (safe for concurrent use).
func (m *MockClient) AllCheckRuns() []*CheckRunRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*CheckRunRecord, 0, len(m.checkRuns))
	for _, cr := range m.checkRuns {
		cp := *cr
		out = append(out, &cp)
	}
	return out
}

// AllCommitStatuses returns a snapshot of all recorded commit status updates.
func (m *MockClient) AllCommitStatuses() []CommitStatusRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CommitStatusRecord, len(m.statuses))
	copy(out, m.statuses)
	return out
}

// CreateCheckRun records and logs a new in_progress check run.
func (m *MockClient) CreateCheckRun(ctx context.Context, installationID int64, repo, name, sha string) (int64, error) {
	id := m.seq.Add(1)
	rec := &CheckRunRecord{
		ID:             id,
		InstallationID: installationID,
		Repo:           repo,
		Name:           name,
		SHA:            sha,
		Status:         CheckStatus{Status: "in_progress"},
	}
	m.mu.Lock()
	m.checkRuns[id] = rec
	m.mu.Unlock()

	log.Printf("[github] CREATE check_run id=%d installation=%d repo=%s name=%q sha=%s status=in_progress",
		id, installationID, repo, name, sha)
	return id, nil
}

// UpdateCheckRun records and logs an updated check run status.
func (m *MockClient) UpdateCheckRun(ctx context.Context, installationID int64, repo string, checkRunID int64, status CheckStatus) error {
	m.mu.Lock()
	rec, ok := m.checkRuns[checkRunID]
	if ok {
		rec.Status = status
	}
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("check run %d not found", checkRunID)
	}

	log.Printf("[github] UPDATE check_run id=%d installation=%d repo=%s status=%s conclusion=%s",
		checkRunID, installationID, repo, status.Status, status.Conclusion)
	return nil
}

// CreateCommitStatus records and logs a commit status update.
func (m *MockClient) CreateCommitStatus(ctx context.Context, installationID int64, repo, sha string, status CommitStatus) error {
	m.mu.Lock()
	m.statuses = append(m.statuses, CommitStatusRecord{
		InstallationID: installationID,
		Repo:           repo,
		SHA:            sha,
		Status:         status,
	})
	m.mu.Unlock()

	log.Printf("[github] COMMIT status installation=%d repo=%s sha=%s state=%s context=%s description=%q",
		installationID, repo, sha, status.State, status.Context, status.Description)
	return nil
}

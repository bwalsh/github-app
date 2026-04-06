package worker_test

import (
	"context"
	"testing"
	"time"

	githubclient "github.com/bwalsh/github-app/internal/github"
	"github.com/bwalsh/github-app/internal/queue"
	"github.com/bwalsh/github-app/internal/worker"
	"github.com/bwalsh/github-app/internal/workflow"
)

// nilResultRunner is a workflow.Runner that returns (nil, nil) to test nil-result handling.
type nilResultRunner struct{}

func (n *nilResultRunner) Run(_ context.Context, _ *workflow.Request) (*workflow.Result, error) {
	return nil, nil
}

type failResultRunner struct{}

func (f *failResultRunner) Run(_ context.Context, _ *workflow.Request) (*workflow.Result, error) {
	return &workflow.Result{Success: false, Message: "workflow failed"}, nil
}

// waitForCompleted polls until all check runs are completed or the timeout expires.
func waitForCompleted(gh *githubclient.MockClient, n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		runs := gh.AllCheckRuns()
		if len(runs) >= n {
			allDone := true
			for _, cr := range runs {
				if cr.Status.Status != "completed" {
					allDone = false
					break
				}
			}
			if allDone {
				return true
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestWorker_ProcessPushDeploy(t *testing.T) {
	q := queue.New(4)
	gh := githubclient.NewMockClient()
	wf := &workflow.StubRunner{Delay: 10 * time.Millisecond}
	w := worker.New(q, gh, wf)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go w.Start(ctx)

	if err := q.Enqueue(&queue.Job{
		Kind:            queue.KindPushDeploy,
		InstallationID:  100,
		RepositoryID:    200,
		RepoFullName:    "org/repo",
		HeadSHA:         "deadbeef",
		Ref:             "refs/heads/main",
		TenantName:      "tenant-100-200",
		TenantNamespace: "ns-200",
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	if !waitForCompleted(gh, 1, 2*time.Second) {
		t.Fatal("timed out waiting for check run to complete")
	}

	runs := gh.AllCheckRuns()
	for _, cr := range runs {
		if cr.Status.Status != "completed" {
			t.Errorf("status: got %q, want completed", cr.Status.Status)
		}
		if cr.Status.Conclusion != "success" {
			t.Errorf("conclusion: got %q, want success", cr.Status.Conclusion)
		}
	}

	statuses := gh.AllCommitStatuses()
	if len(statuses) != 2 {
		t.Fatalf("commit statuses: got %d, want 2", len(statuses))
	}
	if statuses[0].Status.State != "pending" {
		t.Fatalf("first commit status: got %q, want pending", statuses[0].Status.State)
	}
	if statuses[1].Status.State != "success" {
		t.Fatalf("second commit status: got %q, want success", statuses[1].Status.State)
	}
}

func TestWorker_ProcessRepoOnboarding(t *testing.T) {
	q := queue.New(4)
	gh := githubclient.NewMockClient()
	wf := &workflow.StubRunner{Delay: 10 * time.Millisecond}
	w := worker.New(q, gh, wf)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go w.Start(ctx)

	if err := q.Enqueue(&queue.Job{
		Kind:            queue.KindRepoOnboarding,
		InstallationID:  300,
		RepositoryID:    400,
		RepoFullName:    "org/new-repo",
		HeadSHA:         "HEAD",
		TenantName:      "tenant-300-400",
		TenantNamespace: "ns-400",
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	if !waitForCompleted(gh, 1, 2*time.Second) {
		t.Fatal("timed out waiting for check run to complete")
	}

	runs := gh.AllCheckRuns()
	for _, cr := range runs {
		if cr.Name != "repo-onboarding" {
			t.Errorf("check run name: got %q, want repo-onboarding", cr.Name)
		}
	}
}

func TestWorker_NilResultTreatedAsFailure(t *testing.T) {
	q := queue.New(4)
	gh := githubclient.NewMockClient()
	wf := &nilResultRunner{}
	w := worker.New(q, gh, wf)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go w.Start(ctx)

	if err := q.Enqueue(&queue.Job{
		Kind:            queue.KindPushDeploy,
		InstallationID:  500,
		RepositoryID:    600,
		RepoFullName:    "org/repo",
		HeadSHA:         "cafebabe",
		Ref:             "refs/heads/main",
		TenantName:      "tenant-500-600",
		TenantNamespace: "ns-600",
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	if !waitForCompleted(gh, 1, 2*time.Second) {
		t.Fatal("timed out waiting for check run to complete")
	}

	runs := gh.AllCheckRuns()
	for _, cr := range runs {
		if cr.Status.Status != "completed" {
			t.Errorf("status: got %q, want completed", cr.Status.Status)
		}
		if cr.Status.Conclusion != "failure" {
			t.Errorf("conclusion: got %q, want failure", cr.Status.Conclusion)
		}
	}

	statuses := gh.AllCommitStatuses()
	if len(statuses) != 2 {
		t.Fatalf("commit statuses: got %d, want 2", len(statuses))
	}
	if statuses[0].Status.State != "pending" {
		t.Fatalf("first commit status: got %q, want pending", statuses[0].Status.State)
	}
	if statuses[1].Status.State != "failure" {
		t.Fatalf("second commit status: got %q, want failure", statuses[1].Status.State)
	}
}

func TestWorker_PushWorkflowFailureSetsFailureStatus(t *testing.T) {
	q := queue.New(4)
	gh := githubclient.NewMockClient()
	wf := &failResultRunner{}
	w := worker.New(q, gh, wf)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go w.Start(ctx)

	if err := q.Enqueue(&queue.Job{
		Kind:            queue.KindPushDeploy,
		InstallationID:  700,
		RepositoryID:    800,
		RepoFullName:    "org/repo",
		HeadSHA:         "feedface",
		Ref:             "refs/heads/main",
		TenantName:      "tenant-700-800",
		TenantNamespace: "ns-800",
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	if !waitForCompleted(gh, 1, 2*time.Second) {
		t.Fatal("timed out waiting for check run to complete")
	}

	statuses := gh.AllCommitStatuses()
	if len(statuses) != 2 {
		t.Fatalf("commit statuses: got %d, want 2", len(statuses))
	}
	if statuses[0].Status.State != "pending" || statuses[1].Status.State != "failure" {
		t.Fatalf("status sequence: got (%q,%q), want (pending,failure)",
			statuses[0].Status.State, statuses[1].Status.State)
	}
}

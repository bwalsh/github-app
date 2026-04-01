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


package queue_test

import (
	"testing"

	"github.com/bwalsh/github-app/internal/queue"
)

func TestQueue_EnqueueAndDequeue(t *testing.T) {
	q := queue.New(4)
	job := &queue.Job{
		Kind:         queue.KindPushDeploy,
		RepoFullName: "org/repo",
		HeadSHA:      "abc123",
	}
	if err := q.Enqueue(job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := <-q.Jobs()
	if got.RepoFullName != job.RepoFullName {
		t.Errorf("got repo %q, want %q", got.RepoFullName, job.RepoFullName)
	}
}

func TestQueue_Full(t *testing.T) {
	q := queue.New(1)
	job := &queue.Job{Kind: queue.KindPushDeploy}
	if err := q.Enqueue(job); err != nil {
		t.Fatalf("first enqueue failed: %v", err)
	}
	if err := q.Enqueue(job); err == nil {
		t.Error("expected error when queue is full, got nil")
	}
}


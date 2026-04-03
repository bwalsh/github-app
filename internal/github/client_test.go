package github

import (
	"context"
	"sync"
	"testing"
)

func TestNewMockClient_InitiallyEmpty(t *testing.T) {
	m := NewMockClient()
	if got := len(m.AllCheckRuns()); got != 0 {
		t.Fatalf("got %d check runs, want 0", got)
	}
}

func TestCreateCheckRun_AssignsSequentialIDsAndInProgress(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	id1, err := m.CreateCheckRun(ctx, 1, "org/repo", "push-deployment", "sha1")
	if err != nil {
		t.Fatalf("create #1 failed: %v", err)
	}
	id2, err := m.CreateCheckRun(ctx, 1, "org/repo", "push-deployment", "sha2")
	if err != nil {
		t.Fatalf("create #2 failed: %v", err)
	}

	if id1 != 1 || id2 != 2 {
		t.Fatalf("ids: got (%d,%d), want (1,2)", id1, id2)
	}

	runs := m.AllCheckRuns()
	if len(runs) != 2 {
		t.Fatalf("got %d check runs, want 2", len(runs))
	}
	for _, run := range runs {
		if run.Status.Status != "in_progress" {
			t.Fatalf("status: got %q, want %q", run.Status.Status, "in_progress")
		}
		if run.Status.Conclusion != "" {
			t.Fatalf("conclusion: got %q, want empty", run.Status.Conclusion)
		}
	}
}

func TestUpdateCheckRun_UpdatesStatus(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	id, err := m.CreateCheckRun(ctx, 7, "org/repo", "repo-onboarding", "sha")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	status := CheckStatus{Status: "completed", Conclusion: "success"}
	if err := m.UpdateCheckRun(ctx, 7, "org/repo", id, status); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	runs := m.AllCheckRuns()
	if len(runs) != 1 {
		t.Fatalf("got %d check runs, want 1", len(runs))
	}
	if runs[0].Status != status {
		t.Fatalf("status: got %+v, want %+v", runs[0].Status, status)
	}
}

func TestUpdateCheckRun_NotFound(t *testing.T) {
	m := NewMockClient()
	err := m.UpdateCheckRun(context.Background(), 1, "org/repo", 999, CheckStatus{Status: "completed", Conclusion: "failure"})
	if err == nil {
		t.Fatal("expected not found error, got nil")
	}
}

func TestAllCheckRuns_ReturnsCopies(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	id, err := m.CreateCheckRun(ctx, 1, "org/repo", "push-deployment", "sha")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	snapshot := m.AllCheckRuns()
	if len(snapshot) != 1 {
		t.Fatalf("got %d check runs, want 1", len(snapshot))
	}
	snapshot[0].Status = CheckStatus{Status: "completed", Conclusion: "failure"}

	fresh := m.AllCheckRuns()
	if fresh[0].Status.Status != "in_progress" {
		t.Fatalf("mutating snapshot should not change stored record for id=%d", id)
	}
}

func TestCreateCheckRun_ConcurrentUniqueIDs(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	const n = 100
	ids := make(chan int64, n)
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			id, err := m.CreateCheckRun(ctx, int64(i), "org/repo", "check", "sha")
			if err != nil {
				t.Errorf("create failed: %v", err)
				return
			}
			ids <- id
		}(i)
	}
	wg.Wait()
	close(ids)

	seen := make(map[int64]bool, n)
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate id generated: %d", id)
		}
		seen[id] = true
	}
	if len(seen) != n {
		t.Fatalf("got %d unique ids, want %d", len(seen), n)
	}
	if got := len(m.AllCheckRuns()); got != n {
		t.Fatalf("stored check runs: got %d, want %d", got, n)
	}
}

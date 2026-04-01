package workflow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bwalsh/github-app/internal/workflow"
)

func testRequest() *workflow.Request {
	return &workflow.Request{
		Kind:            "push_deploy",
		TenantName:      "tenant-1-2",
		TenantNamespace: "ns-2",
		RepoFullName:    "acme/service",
		Ref:             "refs/heads/main",
		HeadSHA:         "abc123",
	}
}

func TestStubRunner_Run_Success(t *testing.T) {
	r := &workflow.StubRunner{Delay: 5 * time.Millisecond}
	req := testRequest()

	result, err := r.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Run returned nil result")
	}
	if !result.Success {
		t.Fatal("expected success=true")
	}

	wantMessage := "workflow push_deploy completed for acme/service @ abc123"
	if result.Message != wantMessage {
		t.Fatalf("message: got %q, want %q", result.Message, wantMessage)
	}
}

func TestStubRunner_Run_ContextDeadlineExceeded(t *testing.T) {
	r := &workflow.StubRunner{Delay: 50 * time.Millisecond}
	req := testRequest()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	result, err := r.Run(ctx, req)
	if result != nil {
		t.Fatalf("expected nil result on cancellation, got %+v", result)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error: got %v, want %v", err, context.DeadlineExceeded)
	}
}

func TestStubRunner_Run_DefaultDelayWhenZero(t *testing.T) {
	r := &workflow.StubRunner{}
	req := testRequest()

	start := time.Now()
	result, err := r.Run(context.Background(), req)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Run returned nil result")
	}
	if elapsed < 150*time.Millisecond {
		t.Fatalf("expected default delay to be applied; elapsed=%v", elapsed)
	}
	if elapsed > time.Second {
		t.Fatalf("Run took unexpectedly long: %v", elapsed)
	}
}


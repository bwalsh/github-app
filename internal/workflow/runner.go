// Package workflow defines the Runner interface for executing tenant workflows
// and provides a StubRunner for local development and testing.
package workflow

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Request describes a workflow execution triggered by a GitHub event.
type Request struct {
	Kind            string // "push_deploy" or "repo_onboarding"
	TenantName      string
	TenantNamespace string
	RepoFullName    string
	Ref             string
	HeadSHA         string
}

// Result holds the outcome of a workflow run.
type Result struct {
	Success bool
	Message string
}

// Runner is the interface for executing tenant workflows.
type Runner interface {
	Run(ctx context.Context, req *Request) (*Result, error)
}

// StubRunner is a mock Runner that simulates work with a short delay.
// It always succeeds and is safe for use in development and tests.
type StubRunner struct {
	// Delay controls the simulated work duration. Defaults to 200ms.
	Delay time.Duration
}

// Run simulates a workflow run with a configurable delay.
func (s *StubRunner) Run(ctx context.Context, req *Request) (*Result, error) {
	delay := s.Delay
	if delay == 0 {
		delay = 200 * time.Millisecond
	}

	log.Printf("[workflow] START kind=%s tenant=%s repo=%s ref=%s sha=%s",
		req.Kind, req.TenantName, req.RepoFullName, req.Ref, req.HeadSHA)

	select {
	case <-time.After(delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	msg := fmt.Sprintf("workflow %s completed for %s @ %s", req.Kind, req.RepoFullName, req.HeadSHA)
	log.Printf("[workflow] DONE kind=%s tenant=%s repo=%s msg=%q",
		req.Kind, req.TenantName, req.RepoFullName, msg)

	return &Result{Success: true, Message: msg}, nil
}


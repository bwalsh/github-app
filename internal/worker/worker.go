// Package worker consumes jobs from the queue, manages the GitHub Checks API
// lifecycle, and delegates execution to the workflow Runner.
package worker

import (
	"context"
	"log"

	githubclient "github.com/bwalsh/github-app/internal/github"
	"github.com/bwalsh/github-app/internal/queue"
	"github.com/bwalsh/github-app/internal/workflow"
)

// Worker dequeues jobs and processes them: create check → run workflow → update check.
type Worker struct {
	queue    *queue.Queue
	github   githubclient.Client
	workflow workflow.Runner
}

// New creates a Worker wired to the given dependencies.
func New(q *queue.Queue, gh githubclient.Client, wf workflow.Runner) *Worker {
	return &Worker{queue: q, github: gh, workflow: wf}
}

// Start processes jobs from the queue until ctx is cancelled.
// Each job is handled in its own goroutine for async execution.
func (w *Worker) Start(ctx context.Context) {
	log.Println("[worker] started")
	for {
		select {
		case <-ctx.Done():
			log.Println("[worker] shutdown")
			return
		case job := <-w.queue.Jobs():
			go w.process(ctx, job)
		}
	}
}

// process runs the full Checks API lifecycle for a single job.
func (w *Worker) process(ctx context.Context, job *queue.Job) {
	checkName := checkRunName(job.Kind)

	log.Printf("[worker] processing kind=%s tenant=%s repo=%s sha=%s",
		job.Kind, job.TenantName, job.RepoFullName, job.HeadSHA)

	// Step 1: Create check run → status: in_progress
	checkRunID, err := w.github.CreateCheckRun(ctx,
		job.InstallationID, job.RepoFullName, checkName, job.HeadSHA)
	if err != nil {
		log.Printf("[worker] failed to create check run: %v", err)
		return
	}

	// Step 2: Run the (stubbed) workflow
	req := &workflow.Request{
		Kind:         string(job.Kind),
		TenantName:   job.TenantName,
		TenantNS:     job.TenantNamespace,
		RepoFullName: job.RepoFullName,
		Ref:          job.Ref,
		HeadSHA:      job.HeadSHA,
	}

	result, wfErr := w.workflow.Run(ctx, req)

	// Step 3: Update check run → status: completed
	var finalStatus githubclient.CheckStatus
	if wfErr != nil {
		log.Printf("[worker] workflow error: %v", wfErr)
		finalStatus = githubclient.CheckStatus{Status: "completed", Conclusion: "failure"}
	} else if !result.Success {
		log.Printf("[worker] workflow reported failure: %s", result.Message)
		finalStatus = githubclient.CheckStatus{Status: "completed", Conclusion: "failure"}
	} else {
		finalStatus = githubclient.CheckStatus{Status: "completed", Conclusion: "success"}
	}

	if err := w.github.UpdateCheckRun(ctx,
		job.InstallationID, job.RepoFullName, checkRunID, finalStatus); err != nil {
		log.Printf("[worker] failed to update check run %d: %v", checkRunID, err)
	}
}

// checkRunName returns the display name for a check run given the job kind.
func checkRunName(kind queue.Kind) string {
	switch kind {
	case queue.KindPushDeploy:
		return "push-deployment"
	case queue.KindRepoOnboarding:
		return "repo-onboarding"
	default:
		return string(kind)
	}
}


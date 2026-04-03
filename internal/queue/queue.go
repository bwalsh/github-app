// Package queue provides a buffered, channel-based job queue for async
// webhook event processing.
package queue

import "errors"

// Kind represents the type of work in a Job.
type Kind string

const (
	// KindPushDeploy is enqueued when a push event is received on the default branch.
	KindPushDeploy Kind = "push_deploy"
	// KindRepoOnboarding is enqueued when a repository is added to the GitHub App installation.
	KindRepoOnboarding Kind = "repo_onboarding"
)

// Job is a unit of work derived from a normalized GitHub webhook event.
// All data required by the worker is copied in at enqueue time.
type Job struct {
	Kind            Kind
	InstallationID  int64
	RepositoryID    int64
	RepoFullName    string
	HeadSHA         string
	Ref             string
	TenantName      string
	TenantNamespace string
}

// Queue is a buffered, non-blocking job queue backed by a channel.
type Queue struct {
	ch chan *Job
}

// New creates a Queue with the given buffer capacity.
func New(size int) *Queue {
	return &Queue{ch: make(chan *Job, size)}
}

// Enqueue adds a job to the queue. Returns an error immediately if the queue is full.
func (q *Queue) Enqueue(job *Job) error {
	select {
	case q.ch <- job:
		return nil
	default:
		return errors.New("queue is full")
	}
}

// Jobs returns the receive-only channel used by workers to consume jobs.
func (q *Queue) Jobs() <-chan *Job {
	return q.ch
}

# ADR 0002: Multi-Tenant GitHub App — Implementation

## Status
Accepted

## Date
2026-04-01

## Branch
`feature/tenant`

## Context

[ADR 0001](0001-multi-tenant-github-app.md) defined the architecture for a multi-tenant GitHub App
supporting Repo Onboarding and Push Deployment workflows with GitHub Checks API status reporting.
This ADR documents the concrete implementation decisions made to realise that design.

The constraints were:

- **Stdlib only** — no external dependencies added to `go.mod`
- **No real GitHub credentials** at development time — the GitHub API must be mockable end-to-end
- **Async event processing** — the HTTP webhook handler must return quickly; all work happens in a background worker
- **Backward compatibility** — the existing `handler.New(secret)` constructor and all pre-existing tests must continue to pass without modification
- **Interface-first design** — both the GitHub client and the workflow runner are defined as interfaces so real implementations can be dropped in later

---

## Decision

### 1. Package structure

Five new packages were introduced under `internal/`:

```
internal/
  tenant/    – tenant registry
  queue/     – buffered job queue
  github/    – GitHub Checks API client interface + mock
  workflow/  – workflow runner interface + stub
  worker/    – async job processor
```

Each package has a single responsibility and depends only on the packages below it in the list.
No circular dependencies exist.

### 2. Tenant identification — `internal/tenant`

```
type Key struct {
    InstallationID int64
    RepositoryID   int64
}
```

A tenant is resolved by `(installation_id, repository_id)`, matching the ADR 0001 decision.
The `Registry` uses a `sync.RWMutex`-protected map so lookups are non-blocking under concurrent
webhook load.

Tenants are **auto-provisioned** at onboarding time: when a `installation_repositories` event
arrives the handler creates a `Tenant` and registers it — no external configuration required for
the stub implementation.

Auto-provisioned tenant name format: `tenant-<installation_id>-<repo_id>`
Auto-provisioned namespace format:   `ns-<repo_id>`

### 3. Async job queue — `internal/queue`

A buffered Go channel (capacity 256 by default) acts as the queue between the webhook handler
and the worker. Two job kinds are defined:

| `Kind` constant | Trigger |
|---|---|
| `KindPushDeploy` | `push` webhook event |
| `KindRepoOnboarding` | `installation_repositories` event, action `added` |

`Enqueue` is **non-blocking**: it returns an error immediately if the buffer is full, and the
handler responds with `503 Service Unavailable`. This prevents the HTTP layer from ever blocking
on a slow worker.

The `Job` struct is self-contained — all fields the worker needs (SHA, full repo name, tenant
name, namespace) are copied at enqueue time so the worker never re-queries the registry.

### 4. GitHub Checks API — `internal/github`

The `Client` interface exposes two methods matching the ADR 0001 lifecycle:

```go
CreateCheckRun(ctx, installationID, repo, name, sha) (int64, error)
UpdateCheckRun(ctx, installationID, repo, checkRunID, CheckStatus) error
```

`MockClient` is the development/test implementation:
- Uses an `atomic.Int64` sequence to generate deterministic check run IDs
- Stores all records in a `sync.Mutex`-protected map
- Logs every call with `log.Printf` so the full lifecycle is visible in the console
- Exposes `AllCheckRuns() []*CheckRunRecord` for race-free inspection in tests

Swapping `MockClient` for a real GitHub App client requires only implementing the two-method
`Client` interface — no other code changes.

### 5. Workflow runner — `internal/workflow`

The `Runner` interface:

```go
Run(ctx context.Context, req *Request) (*Result, error)
```

`StubRunner` simulates work with a configurable `Delay` (defaults to 200 ms) and always
returns `Result{Success: true}`. The `ctx` is respected so tests can cancel mid-run.

Real workflow engines (Argo, Tekton, etc.) are intended to implement `Runner` without changing
any other package.

### 6. Worker — `internal/worker`

`Worker.Start(ctx)` runs a `select` loop over `queue.Jobs()` and `ctx.Done()`. Each job is
dispatched to a goroutine so multiple jobs can be processed concurrently.

Per-job lifecycle matches ADR 0001 exactly:

```
1. CreateCheckRun → status: in_progress
2. workflow.Runner.Run
3. UpdateCheckRun → status: completed, conclusion: success | failure
```

If `CreateCheckRun` fails the job is dropped with a log line (no retry in this iteration).
If `workflow.Run` returns an error or `Result.Success == false`, the check run is updated with
`conclusion: failure`.

### 7. Handler updates — `internal/handler`

`NewWithDeps(secret, registry, queue)` is the production constructor. The original
`New(secret)` is preserved unchanged for backward compatibility; when the registry is `nil`
the handler falls back to a default tenant so `push` events are still processed.

Two new webhook event types are handled:

| `X-GitHub-Event` | Action filter | Behaviour |
|---|---|---|
| `push` | — | Lookup tenant → enqueue `KindPushDeploy` job |
| `installation_repositories` | `added` | Register tenant in registry → enqueue `KindRepoOnboarding` job |
| `installation_repositories` | other | Log and acknowledge; no job enqueued |
| any other | — | Existing generic handler (log + 200 OK) |

A push event for an `(installation_id, repo_id)` pair that has no registry entry returns
`200 OK` with a message rather than an error, to avoid GitHub retrying the delivery.

### 8. Main wiring — `cmd/github-app`

`main.go` constructs all dependencies and starts the worker:

```go
registry  := tenant.New()
q         := queue.New(256)
ghClient  := github.NewMockClient()
wfRunner  := &workflow.StubRunner{}
w         := worker.New(q, ghClient, wfRunner)

ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
go w.Start(ctx)

h := handler.NewWithDeps(secret, registry, q)
```

`signal.NotifyContext` gives the worker a clean shutdown path on `SIGINT`/`SIGTERM`.

---

## Files changed

| File | Change |
|---|---|
| `internal/tenant/tenant.go` | New — `Key`, `Tenant`, `Registry` |
| `internal/tenant/tenant_test.go` | New — register, lookup, unregister, isolation tests |
| `internal/queue/queue.go` | New — `Kind`, `Job`, `Queue` |
| `internal/queue/queue_test.go` | New — enqueue/dequeue, full-queue tests |
| `internal/github/client.go` | New — `Client` interface, `MockClient` |
| `internal/workflow/runner.go` | New — `Runner` interface, `StubRunner` |
| `internal/worker/worker.go` | New — `Worker`, check-run lifecycle |
| `internal/worker/worker_test.go` | New — push-deploy and repo-onboarding end-to-end tests |
| `internal/handler/handler.go` | Modified — new payload types, `NewWithDeps`, push + onboarding routing |
| `internal/handler/handler_test.go` | Modified — added push and onboarding test cases |
| `cmd/github-app/main.go` | Modified — wires all dependencies, graceful shutdown |

---

## Consequences

### Positive

- All original tests continue to pass unchanged
- The full Checks API lifecycle (create → run → update) is observable in logs with no GitHub credentials
- Each concern is isolated in its own package and behind an interface; real implementations are drop-in
- Worker shuts down cleanly on `SIGINT`/`SIGTERM`
- No external dependencies added to `go.mod`

### Negative / deferred

- **No webhook signature verification** — `GITHUB_WEBHOOK_SECRET` is accepted but not yet validated against the `X-Hub-Signature-256` header
- **No retry logic** — if `CreateCheckRun` or `UpdateCheckRun` fails the job is dropped; a retry queue or dead-letter mechanism is needed for production
- **No persistence** — the tenant registry is in-memory; a restart loses all registrations
- **No real workflow engine** — `StubRunner` always succeeds; integration with Argo/Tekton/etc. is a future step
- **No real GitHub client** — `MockClient` must be replaced before the app can interact with the live Checks API
- **Single worker goroutine** — per-tenant parallelism and backpressure are not yet tuned
- **No `checkRunID` persistence** — if the worker crashes between create and update, the in-progress check run is orphaned

---

## Future work

1. Implement `X-Hub-Signature-256` webhook verification
2. Replace `MockClient` with a real GitHub App client (installation token exchange, REST calls)
3. Persist the tenant registry (database or config file)
4. Add retry / dead-letter handling in the worker
5. Replace `StubRunner` with a real workflow engine adapter
6. Add per-tenant rate-limit awareness

## Review

Findings (ordered by severity)
High — process no longer exits cleanly on SIGINT/SIGTERM (regression)
In cmd/github-app/main.go:36-51, signal.NotifyContext cancels ctx, but http.ListenAndServe is never tied to that context and Server.Shutdown is never called.
Result: first Ctrl+C cancels worker only; server can keep running (default signal termination is intercepted), requiring repeated signals or force-kill.
High — push deployment is triggered for every push ref, not just main
internal/handler/handler.go:78-80 routes all push events into handlePush, and handlePush enqueues deployment unconditionally at internal/handler/handler.go:107-124.
ADR/use-case states “Push to main -> deployment workflow runs.” Current behavior deploys feature branches/tags too.
Medium — onboarding queue drop is silently acknowledged after tenant registration
In internal/handler/handler.go:153-173, tenant mapping is registered first, then Enqueue failure is only logged (still returns 200 overall at :176-177).
This can leave repo “registered” but onboarding workflow never executed, creating inconsistent state without retry/visibility to caller.
Medium — potential panic path if a workflow implementation returns (nil, nil)
In internal/worker/worker.go:66-77, code assumes non-nil result when wfErr == nil and dereferences result.Success at :73.
The interface contract doesn’t enforce non-nil result; a future runner could cause panic.

Open questions / assumptions
Is the intended behavior on onboarding queue pressure best-effort (current) or fail-fast (e.g., 503) like push events?
Should deployment be restricted to exactly refs/heads/main, or configurable (e.g., default branch from payload/config)?
Is unbounded per-job goroutine fan-out in internal/worker/worker.go:35-37 acceptable for expected webhook volume?

Secondary summary
Overall structure is solid (clear package boundaries, good interface seams, tests added for happy paths).
Main risks are around shutdown semantics and event filtering correctness versus ADR behavior.


# PR Review Findings — `feature/tenant`

Date: 2026-04-01

## Findings (ordered by severity)

1. **High — nil result panic risk**
   - In `internal/worker/worker.go:66-77`, `result` is dereferenced in `else if !result.Success` without a nil guard.
   - If `workflow.Run` returns `(nil, nil)`, a worker goroutine can panic and silently drop job processing.

2. **High — graceful shutdown regression**
   - In `cmd/github-app/main.go:36-51`, worker cancellation is signal-aware, but HTTP serving still uses `http.ListenAndServe` directly.
   - `Server.Shutdown` is not invoked, so shutdown is not coordinated and can be abrupt (server may outlive worker cancellation path).

3. **Medium — onboarding queue drop is acknowledged but not surfaced**
   - In `internal/handler/handler.go:153-173` and `internal/handler/handler.go:176-177`, onboarding queue-full errors are logged but the request is still acknowledged.
   - This can create an acknowledged-but-not-executed onboarding path.

4. **Medium — deployment triggered for every pushed ref**
   - In `internal/handler/handler.go:78-80` and `internal/handler/handler.go:107-124`, all `push` events enqueue `queue.KindPushDeploy`.
   - There is no ref filter (for example, `refs/heads/main`), so feature branches/tags can trigger deployment.

## Open questions / assumptions

- Should deployment be restricted to a target ref (for example `refs/heads/main`) or remain all-ref by design?
- For onboarding enqueue failures, should webhook responses become non-2xx (to trigger retries) instead of always accepted?
- Should shutdown guarantee completion/drain of in-flight worker jobs and check-run updates before process exit, or is best-effort acceptable?

## Secondary summary

- Structure and package boundaries are solid.
- Main risks are correctness under shutdown/backpressure and mismatch between accepted events vs guaranteed execution.


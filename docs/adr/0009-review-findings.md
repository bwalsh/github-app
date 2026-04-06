# 0009 - PR Review Findings

Date: 2026-04-06
Reviewer: DevOps/Go review
Scope: All current branch changes (tracked + newly added files)

## Findings (ordered by severity)

### 1) **High** - Branch does not pass `go test ./...` due compile-time symbol collision
- **Impact:** CI/test gate fails; branch is not merge-ready.
- **Evidence:** `go test -count=1 ./...` fails with redeclaration errors.
- **Where:** `internal/observability/observability.go:14-17` and `internal/observability/observability.go:53-65`
- **Details:** `LogInfo`, `LogError`, and `LogDebug` are declared both as constants and as functions in the same package scope.
- **Suggested fix:** Rename either the constants (for example, `LevelInfo`, `LevelError`, `LevelDebug`) or the helper functions (`Info`, `Error`, `Debug`) and update callsites.

### 2) **High** - Real GitHub API client is implemented but not wired into runtime
- **Impact:** App still uses mock GitHub client in production execution path, so check-runs/statuses are not actually posted to GitHub.
- **Where:** `cmd/github-app/main.go:39` and `internal/github/real_client.go`
- **Details:** Runtime currently initializes `githubclient.NewMockClient()` and never switches to `NewRealClient(...)`.
- **Suggested fix:** Add configuration-driven client selection in `main.go` (for example env-based), and default safely for local dev. Add startup log indicating which client is active.

### 3) **Medium** - Local Helm smoke check still expects `400` on unsigned webhook, but handler now returns `401`
- **Impact:** Local validation flow gives false failures and misleads operators.
- **Where:** `Makefile:258-263` (`helm-local-checks`) vs `internal/handler/handler.go:84-89`
- **Details:** Handler now enforces `X-Hub-Signature-256` and returns unauthorized for missing/invalid signature.
- **Suggested fix:** Update `helm-local-checks` to expect `401` for unsigned requests, or send a correctly signed request in that check.

### 4) **Medium** - CI test matrix includes duplicate full-suite execution, increasing runtime without adding distinct signal
- **Impact:** Longer CI cycle and higher flake surface from repeated identical suites.
- **Where:** `.github/workflows/ci.yml:61-63` and `.github/workflows/ci.yml:103-104`
- **Details:** `test` job runs full suite with coverage; `test-verification` runs feature subsets and then runs full `./...` again.
- **Suggested fix:** Keep subset checks in `test-verification`, but drop the final full-suite rerun (or replace with a narrower gate not already covered by `test`).

## Validation performed

```bash
go test -count=1 ./...
```

Observed failure:
- `internal/observability/observability.go`: redeclared identifiers (`LogInfo`, `LogError`, `LogDebug`).

## Open questions

1. Should `RealClient` be enabled by default when required GitHub auth env vars are present, with mock fallback only for local/dev?
2. Do we want local smoke checks to validate signature success path (signed request) rather than unauthorized path?
3. Is the new `test-verification` job intended as a temporary migration aid or a permanent CI stage?

## Secondary notes (non-blocking)

- Endpoint docs are generally improved and aligned with signature validation (`docs/api/endpoints.md`, `README.md`, `docs/deploy/github-webhook-exposure.md`).
- Commit-status flow additions in worker and tests are coherent and improve behavioral coverage.

## Resolution status (2026-04-06)

1. **Resolved** - Observability symbol collision fixed by renaming level constants to `LevelDebug`, `LevelInfo`, `LevelWarn`, `LevelError` in `internal/observability/observability.go`.
2. **Resolved** - Runtime client wiring added in `cmd/github-app/main.go` via `buildGitHubClient(...)`; uses `RealClient` when `GITHUB_TOKEN` is set, otherwise `MockClient`.
3. **Resolved** - `Makefile` local smoke check now expects `401` for unsigned webhook requests in `helm-local-checks`.
4. **Resolved** - Removed redundant full-suite rerun from `.github/workflows/ci.yml` `test-verification` job.

### Post-fix validation

```bash
go test -count=1 ./...
```

Result: all packages pass.



# PR Review Findings

Date: 2026-04-03
Scope: Current branch changes in application runtime, tenant persistence, Makefile automation, and deployment/docs updates.

## Resolved findings

1. **Resolved — `TENANT_PERSISTENCE=sqlite` container compatibility**
   - SQLite persistence no longer shells out to `sqlite3`; it now uses an embedded Go SQLite driver via `database/sql`.
   - Implementation updated in `internal/tenant/tenant.go` and dependency added in `go.mod`/`go.sum` (`modernc.org/sqlite`).
   - Result: `TENANT_PERSISTENCE=sqlite` no longer depends on an external `sqlite3` binary and is compatible with the distroless runtime image.

2. **Resolved — `helm-deploy-internal-test` runtime name resolution for long `FULLNAME_OVERRIDE` values**
   - `Makefile` now resolves deployment and service names at runtime using `app.kubernetes.io/instance=$(HELM_RELEASE)` labels.
   - `helm-deploy-internal-test` no longer assumes static `deployment/$(APP_SERVICE_NAME)` or `svc/$(APP_SERVICE_NAME)` names.
   - Result: long/truncated Helm fullname behavior no longer breaks wait/port-forward steps.

## Open findings

None.

## Open questions

None.

## Verification notes

- `go test -count=1 ./internal/tenant` passed.
- `go test -count=1 ./...` passed.
- `helm lint charts/github-app` passed.
- `helm template github-app charts/github-app` rendered successfully.
- `make -n helm-deploy-internal-test` and `make -n helm-local-checks` show expected command wiring.
- Additional targeted checks confirmed the fullname truncation mismatch path for long `FULLNAME_OVERRIDE` values.

## Brief summary

- The branch is generally strong and well-tested, with improved operator flows and checks.
- Previously identified operational edge cases for sqlite runtime compatibility and internal Helm test name resolution are now resolved.


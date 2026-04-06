# Production Deployment Checklist

**Project:** GitHub App (Multi-Tenant Webhook Processor)  
**Prepared:** 2026-04-06  
**Status:** ⚠️ **NOT READY** — Critical gaps must be addressed first

---

## Pre-Deployment Gates

### Gate 1: Security Review ✅ / ❌
**Status:** ❌ **BLOCKED** — Signature validation missing

- [ ] Webhook signature validation implemented
  - Code: `internal/handler/handler.go` - Add `verifySignature()` function
  - Test: `TestHandleWebhook_ValidSignature_Succeeds`
  - Verify: `X-Hub-Signature-256` header validated before processing
  
- [ ] All signature tests pass
  ```bash
  go test -v ./internal/handler -run Signature
  ```

- [ ] No hardcoded secrets in code
  ```bash
  grep -r "secret\|token\|key" . --include="*.go" | grep -v "os.Getenv" | grep -v "test"
  ```

- [ ] Environment variables documented
  - [ ] `GITHUB_WEBHOOK_SECRET` — Required
  - [ ] `PORT` — Optional (default: 8080)
  - [ ] `TENANT_PERSISTENCE` — Optional (memory/sqlite)

---

### Gate 2: Integration Testing ✅ / ❌
**Status:** ❌ **BLOCKED** — No integration tests

- [ ] HTTP integration tests written
  - File: `tests/integration/http_test.go`
  - Test: `TestWebhookHTTPEndpoint`
  - Verify: Real HTTP server accepts valid webhooks

- [ ] End-to-end webhook flow tests written
  - File: `tests/integration/webhook_e2e_test.go`
  - Test: `TestEndToEndPushToDeploy`
  - Verify: Webhook → Handler → Queue → Worker → GitHub API

- [ ] Error scenario tests written
  - File: `tests/integration/error_scenarios_test.go`
  - Test: `TestMalformedWebhookPayload`, `TestGitHubAPIFailure`
  - Verify: App handles errors gracefully

- [ ] All integration tests pass
  ```bash
  go test -v ./tests/integration
  ```

---

### Gate 3: GitHub API Integration ✅ / ❌
**Status:** ❌ **BLOCKED** — Using mock client only

- [ ] Real GitHub API client implemented
  - File: `internal/github/real_client.go`
  - Methods: `CreateCheckRun`, `UpdateCheckRun`, `CreateCommitStatus`
  - Auth: Uses GitHub App installation token

- [ ] GitHub API tests with credentials
  - File: `tests/integration/github_api_test.go`
  - Test: `TestCreateCheckRunAgainstGitHubAPI` (requires GITHUB_TOKEN)
  - Verify: Check runs appear in real GitHub

- [ ] API error handling tested
  - Transient failures (retry logic)
  - Authentication errors
  - Rate limiting

---

### Gate 4: Load Testing ✅ / ❌
**Status:** ❌ **BLOCKED** — No load tests

- [ ] Queue throughput benchmark
  ```bash
  go test -bench=BenchmarkQueueThroughput ./tests/load
  ```
  - Target: 1000+ enqueues/second
  - Passes: Go code, no panics

- [ ] Concurrent registry access test
  ```bash
  go test -race -v ./tests/load/registry_test.go
  ```
  - Target: 100+ concurrent registrations
  - Passes: No race conditions

- [ ] Worker concurrency test
  - Target: 10 concurrent jobs (per maxConcurrentJobs constant)
  - Passes: No deadlocks, proper job processing

---

### Gate 5: Error Handling ✅ / ❌
**Status:** ❌ **BLOCKED** — Not tested

- [ ] Malformed webhook payload handling
  - Invalid JSON → 400 Bad Request
  - Missing required fields → 400 Bad Request
  - No crashes

- [ ] GitHub API failure handling
  - Network timeout → Log error, don't crash
  - Rate limit (429) → Retry (or log and continue)
  - Authentication error (401) → Log and alert

- [ ] Database failure handling (if using SQLite)
  - Database locked → Retry or return error
  - Connection lost → Reconnect attempt
  - No app crash

- [ ] Queue overflow handling
  - Queue full → Return 503 Service Unavailable
  - Worker consumer gracefully handles pressure

---

### Gate 6: Kubernetes Deployment ✅ / ❌
**Status:** ⚠️ **PARTIAL** — Helm chart exists, needs validation

- [ ] Docker image builds without errors
  ```bash
  docker build -t github-app:test .
  ```

- [ ] Helm chart deploys to Kind cluster
  ```bash
  cd charts/github-app
  helm template . -f values.yaml
  helm install github-app . -f values.yaml --dry-run
  ```

- [ ] App health check passes
  ```bash
  curl -i http://localhost:8080/healthz
  # Expected: 200 OK
  ```

- [ ] Webhook endpoint is accessible
  ```bash
  kubectl port-forward svc/github-app 8080:8080
  curl -i -X POST http://localhost:8080/webhook \
    -H "X-GitHub-Event: ping" \
    -d '{}'
  ```

---

### Gate 7: Observability ✅ / ❌
**Status:** 🟡 **PARTIAL** — Logging works, metrics needed

- [ ] Logs are parseable
  - Format: Consistent, human-readable
  - Levels: Debug, Info, Error
  - Context: Installation ID, Repo, Tenant, Job ID included

- [ ] All log messages are reviewed
  ```bash
  grep -n "log\." cmd/github-app/main.go internal/**/*.go
  ```

- [ ] Structured logging (JSON) - FUTURE
  - [ ] Consider: logrus, zap, or slog
  - [ ] Add: Correlation IDs for tracing

- [ ] Metrics exposed - FUTURE
  - [ ] Webhook count (by event type)
  - [ ] Job processing time
  - [ ] Queue depth
  - [ ] GitHub API latency

- [ ] Health check endpoint working
  ```bash
  curl http://localhost:8080/healthz
  # Expected: "ok"
  ```

---

### Gate 8: Documentation ✅ / ❌
**Status:** ✅ **COMPLETE** — README exists

- [ ] README.md covers:
  - [ ] Installation instructions
  - [ ] Usage examples
  - [ ] Environment variables
  - [ ] Endpoints documentation
  - [ ] Development targets

- [ ] Architecture documentation exists
  - [ ] Data flow diagrams
  - [ ] Component responsibilities
  - [ ] Deployment architecture

- [ ] Operations runbook created (FUTURE)
  - [ ] How to debug webhook issues
  - [ ] How to manually trigger a deployment
  - [ ] How to handle database issues
  - [ ] How to scale the app

- [ ] Deployment guide created (FUTURE)
  - [ ] Prerequisites (Kubernetes version, resources)
  - [ ] Configuration options
  - [ ] Troubleshooting section

---

## Code Quality Checks

- [ ] All tests pass with race detector
  ```bash
  go test -race -v ./...
  ```

- [ ] No lint errors
  ```bash
  go vet ./...
  golangci-lint run ./...  # if available
  ```

- [ ] Coverage acceptable (>80%)
  ```bash
  go test -cover ./...
  ```

- [ ] No TODOs/FIXMEs left for production
  ```bash
  grep -r "TODO\|FIXME" . --include="*.go" --exclude-dir=vendor
  ```

- [ ] Dependencies are current (no known vulnerabilities)
  ```bash
  go list -json -m all | nancy sleuth  # or check govulncheck
  ```

---

## Deployment Prerequisites

### Infrastructure

- [ ] Kubernetes cluster ready
  - Version: 1.24+
  - Resources: 2 CPU, 2GB RAM minimum per pod
  - Storage: Persistent volume for SQLite (if using)

- [ ] Network setup
  - [ ] GitHub webhook URL configured
  - [ ] DNS record points to app endpoint
  - [ ] TLS certificate installed (cert-manager or manual)
  - [ ] Firewall allows inbound HTTPS

- [ ] Monitoring & Logging
  - [ ] Logs collected from stdout/stderr
  - [ ] Alerts configured (app down, webhook failures)
  - [ ] Dashboards created

### GitHub Setup

- [ ] GitHub App created
  - [ ] App ID noted
  - [ ] Webhook secret generated (unique, strong)
  - [ ] Permissions configured (checks, statuses)
  - [ ] Webhook URL configured (https://your-domain/webhook)

- [ ] App installed on test repository
  - [ ] Installation ID noted
  - [ ] Test push to main succeeds

### Database (if using SQLite)

- [ ] Persistent volume claimed
- [ ] Backup strategy defined
- [ ] Recovery procedure tested

---

## Day-1 Operational Checks

### After Deployment

- [ ] App pods are running
  ```bash
  kubectl get pods -l app=github-app
  ```

- [ ] Health check passes
  ```bash
  kubectl exec -it pod/github-app-xxx -- \
    curl http://localhost:8080/healthz
  ```

- [ ] Logs are flowing
  ```bash
  kubectl logs -f deployment/github-app
  ```

- [ ] Test webhook succeeds
  - Push to test repository
  - Verify commit status appears in GitHub UI
  - Verify check run appears in GitHub UI

- [ ] No errors in logs
  ```bash
  kubectl logs deployment/github-app | grep -i error
  ```

---

## Go Live Checklist

### Final Validation

- [ ] All production repositories added to GitHub App
- [ ] All previous gates passed ✅
- [ ] Team trained on:
  - [ ] How to view logs
  - [ ] How to restart the app
  - [ ] How to investigate issues
- [ ] Runbook reviewed by on-call team
- [ ] Rollback procedure documented and tested
- [ ] Communication plan for issues

### Post-Deployment (24 hours)

- [ ] Monitor for errors
- [ ] Verify webhooks are being received
- [ ] Verify commit statuses are being posted
- [ ] Check resource usage (CPU, memory)
- [ ] Check database size (if SQLite)

### Post-Deployment (1 week)

- [ ] Review logs for patterns
- [ ] Verify no error rates spike
- [ ] Performance acceptable (latency < 1s per webhook)
- [ ] Scaling needs identified
- [ ] Document lessons learned

---

## Rollback Plan

### If Critical Issue Found

1. [ ] Disable webhook in GitHub App settings (pause new events)
2. [ ] Scale down Kubernetes deployment to 0
3. [ ] Verify no new deployments are triggered
4. [ ] Fix issue (or rollback to previous version)
5. [ ] Test fix in staging
6. [ ] Redeploy to production
7. [ ] Re-enable webhook in GitHub App settings
8. [ ] Monitor for recovery

### Timeline: 5-15 minutes

---

## Sign-Off

- [ ] DevOps Lead: _____________________ Date: _______
- [ ] Security Team: ___________________ Date: _______
- [ ] Product Owner: ___________________ Date: _______
- [ ] Engineering Lead: ________________ Date: _______

---

## Notes

Use this section to document any deviations or special considerations:

_________________________________________________________________

_________________________________________________________________

_________________________________________________________________

---

**Last Updated:** 2026-04-06  
**Next Review:** After Phase 1 implementation  
**Owner:** DevOps SME


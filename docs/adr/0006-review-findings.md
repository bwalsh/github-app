# PR Review Findings – `feature/ci`

**Branch:** `feature/ci` → `main`  
**Commits reviewed:** `9332774`, `3afe20d`, `7e4e9cf` ("adds actions", two gofmt passes)  
**Reviewed:** 2026-04-03  
**Reviewer:** GitHub Copilot

---

## Summary of changes

| File | Change type |
|------|-------------|
| `.github/workflows/ci.yml` | **New** – GitHub Actions CI workflow (test + integration jobs) |
| `docs/deploy/helm-operator-guide.md` | **Updated** – Added section 7 (CI operations & troubleshooting) |
| `internal/queue/queue.go` | `gofmt` – struct field alignment + trailing newline removed |
| `internal/github/client.go` | `gofmt` – trailing newline removed |
| `internal/github/client_test.go` | `gofmt` – trailing newline removed |
| `internal/queue/queue_test.go` | `gofmt` – trailing newline removed |
| `internal/tenant/tenant.go` | `gofmt` – trailing newline removed |
| `internal/version/version_test.go` | `gofmt` – trailing newline removed |
| `internal/worker/worker_test.go` | `gofmt` – trailing newline removed |
| `internal/workflow/runner.go` | `gofmt` – trailing newline removed |
| `internal/workflow/runner_test.go` | `gofmt` – trailing newline removed |

---

## 🔴 Critical

### C1 – `Verify formatting` step is missing the YAML block-scalar indicator (`|`)

**File:** `.github/workflows/ci.yml`, `Verify formatting` step  

The `run:` key does not use a `|` block-scalar, so the multi-line shell script is **not a valid YAML scalar**. GitHub Actions will either reject the workflow with a parse error or collapse the lines into a single token that the shell cannot interpret.

```yaml
# Current (broken)
      - name: Verify formatting
        run: 
          unformatted="$(gofmt -l .)"
          if [ -n "$unformatted" ]; then
            ...
          fi

# Required fix
      - name: Verify formatting
        run: |
          unformatted="$(gofmt -l .)"
          if [ -n "$unformatted" ]; then
            echo "The following files are not gofmt-formatted:"
            echo "$unformatted"
            exit 1
          fi
```

**Impact:** The `test` job will fail to parse/execute, blocking the entire workflow on every push and pull request.  
**Recommendation:** Add `|` after `run:` on this step.

---

## 🟠 High

### H1 – `test` job has no `timeout-minutes` ✅ RESOLVED

**File:** `.github/workflows/ci.yml`, `test` job

The `integration` job correctly sets `timeout-minutes: 45`, but the `test` job has no timeout. A stuck test (e.g., a goroutine deadlock not caught by the race detector) could hold a runner for the GitHub Actions default of **6 hours**.

```yaml
  test:
    runs-on: ubuntu-latest
    timeout-minutes: 15   # add this
```

**Recommendation:** Set `timeout-minutes: 15` (or an appropriate ceiling) on the `test` job.

**Resolution:** Added `timeout-minutes: 15` to the `test` job.

---

### H2 – `go mod verify` is absent from the workflow ✅ RESOLVED

**File:** `.github/workflows/ci.yml`, `test` job

`go mod download` fetches dependencies but does not validate their checksums against `go.sum`. A tampered or corrupted module cache will silently produce untrusted builds.

```yaml
      - name: Verify module checksums
        run: go mod verify
```

**Recommendation:** Add a `go mod verify` step immediately after `Download dependencies`.

**Resolution:** Added `Verify module checksums` step (`go mod verify`) immediately after `Download dependencies`.

---

## 🟡 Medium

### M1 – GitHub Actions version tags are mutable; pin to commit SHAs

**File:** `.github/workflows/ci.yml`

All four action references use mutable version tags:

| Action | Current ref | Risk |
|--------|-------------|------|
| `actions/checkout` | `@v4` | Tag can be force-pushed |
| `actions/setup-go` | `@v5` | Tag can be force-pushed |
| `azure/setup-helm` | `@v4` | Tag can be force-pushed |
| `helm/kind-action` | `@v1` | Tag can be force-pushed |

Supply-chain attacks via tag mutation are a documented, real-world risk. OSSF / GitHub hardening guides recommend pinning to the full commit SHA with the tag as a comment:

```yaml
uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4
```

**Recommendation:** Pin all four actions to their current commit SHAs. Tools like `pinact` or Dependabot (with `update-type: version-update:semver-patch` disabled for actions) can automate this.

---

### M2 – `azure/setup-helm@v4` duplicates Helm installation already done by `helm/kind-action@v1` ✅ RESOLVED

**File:** `.github/workflows/ci.yml`, `integration` job

`helm/kind-action@v1` installs Helm as part of its setup. Adding `azure/setup-helm@v4` afterwards installs a second, potentially different version. This adds unnecessary installation time and can introduce subtle version skew between the Helm version used to validate the chart and the one used to deploy.

**Recommendation:** Remove the `Set up Helm` step from the `integration` job and rely solely on the version bundled with `helm/kind-action@v1`, or—if a specific Helm version is required—set it via `helm/kind-action`'s `helm_version` input and remove `azure/setup-helm`.

**Resolution:** Removed the redundant `Set up Helm` step from the `integration` job so `helm/kind-action@v1` is the only action responsible for installing Helm.

---

### M3 – Hardcoded plain-text webhook secret in workflow env block ✅ RESOLVED

**File:** `.github/workflows/ci.yml`, `Run Kind integration deploy/verify` step

```yaml
        env:
          GITHUB_WEBHOOK_SECRET: ci-integration-secret
```

Using a literal string is acceptable for an ephemeral integration test that never contacts real GitHub, but it will appear in workflow run logs and in the workflow YAML in plaintext. If this repository is ever used as a template, the pattern may be copied into contexts where the value is more sensitive.

**Recommendation:** Move the value to a GitHub Actions repository secret (`${{ secrets.CI_WEBHOOK_SECRET }}`) and document in the workflow comment that its value can be any non-empty string for integration testing purposes. This also makes it easier to rotate if the deployment model changes.

**Resolution:** Replaced the hardcoded value with `${{ secrets.CI_WEBHOOK_SECRET }}` and added a workflow comment explaining that any non-empty repository secret value is sufficient for this ephemeral CI integration test.

---

### M4 – No `go mod tidy` check; `go.sum` drift is undetected ✅ RESOLVED

**File:** `.github/workflows/ci.yml`, `test` job

`go mod download` and `go mod verify` catch dependency integrity issues but do not catch a `go.mod`/`go.sum` that is out of sync with the source (e.g., an import added without running `go mod tidy`). The build still succeeds in that case because all required modules are present, but the repo state is inconsistent.

```yaml
      - name: Check go mod tidy
        run: |
          go mod tidy
          git diff --exit-code go.mod go.sum
```

**Recommendation:** Add the step above to catch `go.mod`/`go.sum` drift in CI.

**Resolution:** Added a `Check go mod tidy` step that runs `go mod tidy` followed by `git diff --exit-code -- go.mod go.sum`.

---

## 🔵 Low / Enhancement

### L1 – No test coverage reporting ✅ RESOLVED

**File:** `.github/workflows/ci.yml`, `test` job

The workflow runs `go test -race -count=1 ./...` but does not collect or upload a coverage report. The `Makefile` already has a `coverage` target that produces `coverage.out`. Uploading to Codecov, Coveralls, or even as a GitHub Actions artifact would give reviewers trend visibility.

**Recommendation (optional):** Replace the `Run tests` step with:

```yaml
      - name: Run tests with coverage
        run: go test -race -count=1 -coverprofile=coverage.out -covermode=atomic ./...

      - name: Upload coverage artifact
        uses: actions/upload-artifact@v4
        with:
          name: coverage
          path: coverage.out
```

**Resolution:** Replaced the plain `Run tests` step with a coverage-producing test step and added an `Upload coverage artifact` step for `coverage.out`.

---

### L2 – Linting is limited to `go vet` ✅ RESOLVED

**File:** `.github/workflows/ci.yml`, `test` job

`go vet` catches only a small subset of common issues. The project has no `golangci-lint` configuration yet, so adding it now alongside `go vet` would provide substantially better static analysis (e.g., `staticcheck`, `errcheck`, `gosec`) without breaking any existing code.

**Recommendation:** Add `golangci-lint` via `golangci-lint-action@v6` as a separate step or job.

**Resolution:** Added a `Run golangci-lint` step using `golangci/golangci-lint-action@v6` and committed a conservative `.golangci.yml` configuration validated against the current codebase.

---

### L3 – `helm-operator-guide.md` section 7.2 step 4 is slightly imprecise ✅ RESOLVED

**File:** `docs/deploy/helm-operator-guide.md`, section 7.2

Step 4 states:

> Installs/updates the Helm release and verifies `/healthz` through port-forward.

The verb "installs/updates" comes from `make kind-deploy-local` (called inside `scripts/kind-deploy-verify.sh`). The health check and port-forward are done by the script itself, not by the Helm target. The current wording conflates the two, which could confuse a reader trying to reproduce a failure.

**Recommendation:** Split into two steps for clarity:

> 4. Deploys the Helm release via `make kind-deploy-local`.
> 5. Port-forwards the service and verifies `/healthz` via `scripts/kind-deploy-verify.sh`.

**Resolution:** Updated section 7.2 to split Helm deployment from the script-driven port-forward and `/healthz` verification steps.

---

### L4 – `integration` job checkout/setup-go steps duplicate the `test` job exactly ✅ RESOLVED

**File:** `.github/workflows/ci.yml`

Both jobs perform identical `Check out repository` and `Set up Go` steps. While duplication is unavoidable for separate jobs, the Go setup in `integration` is not used beyond what `make kind-deploy-verify` (which calls `docker build` and the Makefile's Go targets) requires. Confirming that `go-version-file` is actually needed in `integration` (vs. relying on the system Go installed by `helm/kind-action`'s Ubuntu runner) would avoid an unnecessary download-and-cache cycle.

**Recommendation:** Evaluate whether `Set up Go` is strictly required in the `integration` job. If the Docker build compiles Go inside the container (multi-stage `Dockerfile`), the step may be redundant.

**Resolution:** Removed `Set up Go` from the `integration` job after confirming `make kind-deploy-verify` relies on Docker/Kind/Helm/kubectl on the runner and that the application binary is compiled inside the multi-stage `Dockerfile` build.

---

## ✅ Positive observations

- **`if: always()` on cleanup** – `make kind-clean` runs even when preceding steps fail. This correctly prevents orphaned Kind clusters on the CI runner.
- **`needs: test` sequencing** – The integration job correctly gates on the test job, avoiding wasted runner time when unit tests fail.
- **Race detector enabled** – `go test -race` is a strong default that catches real concurrency bugs early.
- **`go-version-file: go.mod`** – Sourcing the Go version from `go.mod` is better than hardcoding it in the workflow.
- **`gofmt` cleanup** – The trailing-newline and struct-alignment normalisation across 9 files is clean housekeeping; these changes are purely cosmetic and do not affect behaviour.
- **Documentation section 7** – The troubleshooting playbook in `helm-operator-guide.md` is detailed, maps symptoms to fixes, and includes safe-rerun guidance. This is high-quality operational documentation.
- **`permissions: contents: read`** – Minimal top-level permissions are a security best practice.

---

## Action items (prioritised)

| Priority | Item | File |
|----------|------|------|
| 🔴 | Add `|` to `Verify formatting` `run:` block | `ci.yml` |
| ~~🟠~~ | ~~Add `timeout-minutes` to `test` job~~ ✅ | `ci.yml` |
| ~~🟠~~ | ~~Add `go mod verify` step~~ ✅ | `ci.yml` |
| 🟡 | Pin action refs to commit SHAs | `ci.yml` |
| ~~🟡~~ | ~~Remove redundant `azure/setup-helm` step~~ ✅ | `ci.yml` |
| ~~🟡~~ | ~~Move webhook secret to GitHub Actions secret~~ ✅ | `ci.yml` |
| ~~🟡~~ | ~~Add `go mod tidy` drift check~~ ✅ | `ci.yml` |
| ~~🔵~~ | ~~Add coverage collection & upload~~ ✅ | `ci.yml` |
| ~~🔵~~ | ~~Add `golangci-lint`~~ ✅ | `ci.yml` |
| ~~🔵~~ | ~~Clarify section 7.2 step 4 in docs~~ ✅ | `helm-operator-guide.md` |
| ~~🔵~~ | ~~Evaluate whether `Set up Go` is needed in `integration` job~~ ✅ | `ci.yml` |


# PR Review Findings - `feature/badges`

**Branch:** `feature/badges` -> `main`  
**Commits reviewed:** `9332774`, `3afe20d`, `7e4e9cf`, `0e73010`, `2b10b0a`, `6d07939`, `2c93ca7`, `f74d291`, `89d817b`, `1d3762b`  
**Reviewed:** 2026-04-04

---

## Findings (ordered by severity)

### Medium

1. **GitHub Actions uses mutable action tags instead of pinned commit SHAs**
   - **Why it matters:** mutable tags can be retargeted, creating avoidable CI supply-chain risk.
   - **References:**
     - `.github/workflows/ci.yml:19` (`actions/checkout@v4`)
     - `.github/workflows/ci.yml:22` (`actions/setup-go@v5`)
     - `.github/workflows/ci.yml:51` (`golangci/golangci-lint-action@v7`)
     - `.github/workflows/ci.yml:59` (`actions/upload-artifact@v4`)
     - `.github/workflows/ci.yml:74` (`actions/checkout@v4`)
     - `.github/workflows/ci.yml:78` (`helm/kind-action@v1`)
   - **Recommendation:** pin each `uses:` reference to a full commit SHA (optionally keep the tag in a trailing comment).

### Low

1. **`docs/adr/0006-review-findings.md` contains stale details relative to this branch**
   - **Why it matters:** the ADR is being used as a running status artifact (`RESOLVED` annotations), so stale entries can mislead future triage.
   - **References:**
     - `docs/adr/0006-review-findings.md:110` still lists `azure/setup-helm` in the M1 action table even though it was removed from `.github/workflows/ci.yml`.
     - `docs/adr/0006-review-findings.md:30` still characterizes non-`|` `run:` blocks as invalid YAML; current workflow behavior parses those blocks as multi-line strings.
   - **Recommendation:** either refresh `0006` to match the implemented state or add an explicit note that the document is a historical snapshot and not a live status ledger.

---

## Open Questions / Assumptions

- Assumed this review scope is the committed branch delta (`main...HEAD`). Local uncommitted edits in the working tree were not treated as part of the PR.
- Assumed `docs/adr/0006-review-findings.md` is intended to be maintained over time because it already tracks `RESOLVED` status.

---

## Validation Performed

- `go test -count=1 ./...` (pass)
- YAML parse check for committed workflow via Ruby/Psych (pass)
- `golangci-lint v2.11.4` with repository `.golangci.yml` (pass)

---

## Brief Summary

- No functional regressions were identified in the changed Go runtime code; code changes outside CI/docs are cosmetic `gofmt` updates.
- Primary actionable issue is CI hardening by pinning action SHAs.
- One low-priority documentation consistency issue remains in `docs/adr/0006-review-findings.md`.

# PR Review Findings - `bug/badges`

**Branch:** `bug/badges` -> `main`  
**Commits reviewed:** `5d6ec9c`  
**Reviewed:** 2026-04-06

---

## Findings (ordered by severity)

### Medium

1. **GitHub Actions uses mutable `uses:` tags instead of pinned commit SHAs** ✅ RESOLVED
   - **Resolution evidence:**
     - `.github/workflows/ci.yml:19`, `.github/workflows/ci.yml:22`, `.github/workflows/ci.yml:51`, `.github/workflows/ci.yml:59`, `.github/workflows/ci.yml:76`, `.github/workflows/ci.yml:79`, `.github/workflows/ci.yml:131`, `.github/workflows/ci.yml:135`
   - **Resolution note:** action refs are now pinned to full commit SHAs with version comments.

2. **`contents: write` permission is granted for the whole `test` job on all triggers** ✅ RESOLVED
   - **Resolution evidence:**
     - `test` job no longer declares write permission (`.github/workflows/ci.yml:13-65`)
     - dedicated `coverage-badge` job has `permissions: contents: write` and is push-only (`.github/workflows/ci.yml:67-73`)
   - **Resolution note:** write scope is now isolated to the badge update path on `push` to `main`.

---

## Open Questions / Assumptions

- Assumed direct pushes from `github-actions[bot]` to `main` are allowed by branch protection settings for the `coverage-badge` job.

---

## Validation Performed

- `go test -count=1 ./...` (pass)
- YAML parse check for `.github/workflows/ci.yml` via Ruby/Psych (pass)
- Confirmed no mutable action tags remain (`grep "uses: .*@v" .github/workflows/ci.yml` returned no matches)

---

## Brief Summary

- Both medium findings have been resolved.
- Coverage badge automation remains endpoint-backed and self-updating via `.github/badges/coverage.json`.
- No open findings remain for this review.

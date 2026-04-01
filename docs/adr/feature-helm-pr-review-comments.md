# PR Review Comments

Date: 2026-04-01
Scope: Current branch changes (Go app, Helm chart, Makefile, Dockerfile, deployment docs)

## Findings (ordered by severity)

1. **High — Docker build toolchain version mismatch**
   - `Dockerfile:1` uses `golang:1.21` while `go.mod:3` requires Go `1.24.13`.
   - This can break container builds in CI/local with unsupported Go version errors.

2. **Medium — Secret creation flow is brittle for multiline private keys and exposes secrets in args**
   - `Makefile:138-143` uses `kubectl create secret ... --from-literal=github-app-private-key="$(GITHUB_APP_PRIVATE_KEY)"`.
   - `docs/deploy/kind-helm-cert-manager.md:46` instructs loading PEM into an env var first.
   - Multiline PEM via CLI literals is error-prone and can expose secret contents through process arguments.

3. **Medium — Ingress controller install is unpinned**
   - `Makefile:122` installs ingress-nginx from the `main` branch URL.
   - This makes bootstrap non-deterministic and can cause unexpected breakage over time.

## Open questions

1. Should `kind-create-secrets` switch private key handling to `--from-file` and fail fast if required values are missing?
2. Should ingress-nginx installation be pinned to a specific release manifest?
3. Is Docker image build part of the CI/release path for this branch (so Go base image alignment is required immediately)?

## Verification notes

- `go test ./...` passes in current workspace.
- `helm lint charts/github-app` passes.
- `helm template` renders with default values and local fallback issuer settings.

## Brief summary

- Core app/test changes are in good shape.
- Main risks are deployment reproducibility and secret handling ergonomics/security.


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

4. **High — `helm-status` assumes ingress name equals Helm release name**
   - `Makefile:186` runs `kubectl describe ingress $(HELM_RELEASE)`.
   - The chart ingress name is generated via `github-app.fullname` (release + chart name by default), so this often resolves to a different object name.
   - Impact: `make helm-status` can fail even when ingress is healthy, which can mislead operators during post-deploy checks.

## Review update (Makefile + `docs/deploy/helm-operator-guide.md`)

- Documentation readability is strong overall: clear prerequisites, step-by-step command blocks, and practical explanations for secret sourcing and TLS modes.
- Main correctness gap remains in `helm-status` ingress lookup naming (`Makefile:186`).
- Guide command examples in `docs/deploy/helm-operator-guide.md` align with current Make targets (`k8s-*`, `helm-*`).

## Open questions

1. Should `kind-create-secrets` switch private key handling to `--from-file` and fail fast if required values are missing?
2. Should ingress-nginx installation be pinned to a specific release manifest?
3. Is Docker image build part of the CI/release path for this branch (so Go base image alignment is required immediately)?
4. Should `helm-status` switch to label-based ingress selection (for example `app.kubernetes.io/instance=$(HELM_RELEASE)`) to avoid name-coupling?
5. Should we document the generated ingress name behavior (`fullnameOverride` vs default release+chart naming) in `docs/deploy/helm-operator-guide.md`?

## Verification notes

- `go test ./...` passes in current workspace.
- `helm lint charts/github-app` passes.
- `helm template` renders with default values and local fallback issuer settings.

## Brief summary

- Core app/test changes are in good shape.
- Main risks are deployment reproducibility and secret handling ergonomics/security.


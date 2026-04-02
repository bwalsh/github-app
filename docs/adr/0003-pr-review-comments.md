# PR Review Comments

Status: Updated to reflect current branch as of 2026-04-02. Original mismatches called out below have been fixed.

Date: 2026-04-01
Scope: Current branch changes (Go app, Helm chart, Makefile, Dockerfile, deployment docs)

## Findings (ordered by severity)

1. **Resolved — Docker build toolchain version alignment**
   - `Dockerfile:1` now uses `golang:1.24.13`, matching the Go version `1.24.13` required by `go.mod:3`.
   - The earlier risk of container builds failing due to an outdated Go base image has been addressed.

2. **Resolved — Secret creation flow hardened for multiline private keys**
   - `Makefile:138-143` now creates the GitHub App private key secret via `kubectl create secret ... --from-file=github-app-private-key=<path-to-pem>`.
   - `docs/deploy/kind-helm-cert-manager.md:46` has been updated to instruct storing the PEM on disk and mounting it from a file.
   - This avoids multiline-PEM parsing issues via CLI literals and reduces exposure of secret contents through process arguments.

3. **Resolved — Ingress controller install is now pinned**
   - `Makefile:122` now installs ingress-nginx from a pinned release manifest rather than the moving `main` branch URL.
   - This makes bootstrap deterministic and reduces the risk of unexpected breakage over time.

4. **Resolved — `helm-status` now uses label-based ingress lookup**
   - `Makefile:186` now runs `kubectl get ingress -l app.kubernetes.io/instance=$(HELM_RELEASE)` (or equivalent) instead of assuming the ingress name equals the Helm release name.
   - The chart ingress name is still generated via `github-app.fullname` (release + chart name by default), but the label-based lookup correctly finds the object regardless of the generated name.
   - Impact: previous `make helm-status` failures due to ingress name mismatch are resolved; operators now see accurate status as long as ingress resources carry the expected labels.

## Review update (Makefile + `docs/deploy/helm-operator-guide.md`)

- Documentation readability is strong overall: clear prerequisites, step-by-step command blocks, and practical explanations for secret sourcing and TLS modes.
- No remaining correctness gaps were identified in `helm-status` ingress lookup for the current branch.
- Guide command examples in `docs/deploy/helm-operator-guide.md` align with current Make targets (`k8s-*`, `helm-*`).

## Open questions

1. Is Docker image build part of the CI/release path for this branch (to determine how strictly we need to gate on base image upgrades in future)?
2. Should we further document the generated ingress name behavior (`fullnameOverride` vs default release+chart naming) and the label-based lookup pattern in `docs/deploy/helm-operator-guide.md`?

## Verification notes

- `go test ./...` passes in current workspace.
- `helm lint charts/github-app` passes.
- `helm template` renders with default values and local fallback issuer settings.

## Brief summary

- Core app/test changes are in good shape.
- Main risks are deployment reproducibility and secret handling ergonomics/security.


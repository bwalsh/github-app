# Operator guide: installing and operating the `github-app` Helm chart

This guide is for cluster operators deploying `charts/github-app` into Kubernetes environments outside the local Kind bootstrap flow.

If you need a one-command local Kind smoke test, use `make kind-deploy-verify` (wrapper around `scripts/kind-deploy-verify.sh`) and refer to `docs/deploy/kind-helm-cert-manager.md`.

## 1) Server prerequisites

Before installing the chart, make sure the target cluster and operator workstation have the following:

### Kubernetes and tooling

- Kubernetes cluster (v1.22+ recommended so `networking.k8s.io/v1` Ingress is fully supported).
- `kubectl` configured for the target cluster context.
- Helm v3.

### Cluster capabilities

- A working Ingress controller (default chart value expects `ingress.className: nginx`).
- DNS record for your webhook host that resolves to the ingress endpoint.
- If using cert-manager automation:
  - `cert-manager` installed.
  - A `ClusterIssuer` available (for example `letsencrypt-staging`, `letsencrypt-production`, or `selfsigned-local`).

### Application secrets

The chart expects a Kubernetes Secret with these keys:

- `github-webhook-secret` (required)
- `github-app-id` (optional)
- `github-app-installation-id` (optional)
- `github-app-private-key` (optional)

> The Deployment references these through `secrets.githubWebhookSecretRef` and `secrets.githubAppRef` values.

---

## 2) Create namespace and required secret

```bash
make k8s-namespace K8S_NAMESPACE=github-app
```

Create/update the secret consumed by the chart:

```bash
GITHUB_WEBHOOK_SECRET='replace-me' \
GITHUB_APP_ID='123456' \
GITHUB_APP_INSTALLATION_ID='654321' \
GITHUB_APP_PRIVATE_KEY_FILE='/path/to/github-app.private-key.pem' \
make k8s-create-secrets K8S_NAMESPACE=github-app
```

Behavior of `make k8s-create-secrets`:

- `GITHUB_WEBHOOK_SECRET` is required.
- `GITHUB_APP_ID` and `GITHUB_APP_INSTALLATION_ID` are optional, but the target includes them only when **both** are provided.
- If only one of `GITHUB_APP_ID` or `GITHUB_APP_INSTALLATION_ID` is set, the target prints a warning and skips both keys.
- `GITHUB_APP_PRIVATE_KEY_FILE` is preferred; `GITHUB_APP_PRIVATE_KEY` is accepted as a compatibility fallback.

If you use different key names or a different secret name, override `secrets.*` values at install time.

### Where these values come from in GitHub

Use your **GitHub App** settings page to collect the values before creating `github-app-secrets`:

- `github-webhook-secret`
  - In GitHub: **GitHub App > Webhook > Webhook secret**.
  - Use the same exact value in Kubernetes; GitHub sends signatures derived from this value.
- `github-app-id`
  - In GitHub: **GitHub App > General > App ID**.
- `github-app-installation-id`
  - In GitHub: open the installed app target and copy the numeric installation ID from the installation URL.
- `github-app-private-key`
  - In GitHub: **GitHub App > General > Private keys > Generate a private key**.
  - GitHub downloads a `.pem` file once at generation time; store it securely and load it with `--from-file`.

### What happens if values are not provided

- If `github-webhook-secret` key (or the backing Secret) is missing, Kubernetes cannot resolve `GITHUB_WEBHOOK_SECRET` and the pod will fail to start (`CreateContainerConfigError`).
- `github-app-id`, `github-app-installation-id`, and `github-app-private-key` are marked optional in the Deployment, so pods can still start if they are absent.
- Current application behavior: webhook handling still runs without app credentials because event processing in this service does not currently require those three values.
- Security note: this codebase currently does not enforce webhook signature verification in the handler; set and manage `github-webhook-secret` anyway so future signature validation can be enabled without secret migration.

### Secret key mapping used by `k8s-create-secrets`

- `github-webhook-secret` <- `GITHUB_WEBHOOK_SECRET` (required)
- `github-app-id` <- `GITHUB_APP_ID` (included only when paired with installation ID)
- `github-app-installation-id` <- `GITHUB_APP_INSTALLATION_ID` (included only when paired with app ID)
- `github-app-private-key` <- `GITHUB_APP_PRIVATE_KEY_FILE` (preferred) or `GITHUB_APP_PRIVATE_KEY`

---

## 3) Install and configure the Helm chart

From the repository root:

```bash
make helm-deploy-production \
  K8S_NAMESPACE=github-app \
  HELM_RELEASE=github-app \
  IMG_REPO=ghcr.io/<org>/<image> \
  IMG_TAG=<tag> \
  HOST=webhooks.example.com \
  TLS_SECRET=github-app-webhooks-tls
```

Recommended: commit environment-specific values files (without secrets) such as:

- `values-staging.yaml`
- `values-production.yaml`

Then install with:

```bash
helm upgrade --install github-app ./charts/github-app --namespace github-app -f values-production.yaml
```

### 3.1 Bring up an internal-only test deployment (no public internet)

Use this flow when you need to validate the app in a disconnected/private environment where GitHub cannot call into the cluster.

1. Deploy the chart with ingress disabled (no public endpoint required), wait for pods, and start port-forward:

```bash
make helm-deploy-internal-test \
  K8S_NAMESPACE=github-app \
  HELM_RELEASE=github-app
```

2. In another terminal, run local checks against the port-forward:

```bash
make helm-local-checks LOCAL_PORT=8080
```

Notes:

- This mode is for operator and chart validation only; GitHub webhook delivery requires a publicly reachable HTTPS endpoint.
- If you still want to exercise ingress/TLS objects without public internet, use a private host plus `certManager.localFallbackIssuer.enabled=true` with the `selfsigned-local` ClusterIssuer (see `kind-deploy-local` pattern in `docs/deploy/kind-helm-cert-manager.md`).
- Service naming follows the same Helm fullname behavior as other resources: by default `<release>-<chart>` (for example `github-app-github-app`). If `fullnameOverride` is set, use that value instead.

---

## 4) Supplying your own ingress public/private key pair

If your organization manages TLS certificates outside cert-manager, provide your own certificate and private key as a TLS secret and configure ingress to use it.

### 4.1 Create TLS secret from your cert/key

```bash
TLS_CERT_FILE=/path/to/tls.crt \
TLS_KEY_FILE=/path/to/tls.key \
make k8s-create-tls-secret \
  K8S_NAMESPACE=github-app \
  TLS_SECRET=github-app-webhooks-tls
```

- `tls.crt` is your public certificate chain.
- `tls.key` is your private key.

### 4.2 Disable cert-manager integration and reference the secret

Use Helm overrides:

```bash
make helm-deploy-local-tls \
  K8S_NAMESPACE=github-app \
  HELM_RELEASE=github-app \
  IMG_REPO=ghcr.io/<org>/<image> \
  IMG_TAG=<tag> \
  HOST=webhooks.example.com \
  TLS_SECRET=github-app-webhooks-tls
```

With `certManager.enabled=false`, the chart will not set `cert-manager.io/cluster-issuer` on the Ingress and your pre-created TLS secret will be used directly.

---

## 5) Post-install checks

```bash
make helm-status \
  K8S_NAMESPACE=github-app \
  HELM_RELEASE=github-app \
  TLS_SECRET=github-app-webhooks-tls
```

Validate service health:

- In-cluster probe path is `/healthz`.
- GitHub webhook endpoint path is `/webhook`.

### Ingress resource naming (important for troubleshooting)

The chart sets ingress `metadata.name` with `{{ include "github-app.fullname" . }}`.

- Default behavior: ingress name is `<release-name>-<chart-name>` (for this chart, typically `$(HELM_RELEASE)-github-app`).
- If `fullnameOverride` is set, ingress name becomes exactly that override value.
- Because of this, do **not** assume the ingress is named exactly `$(HELM_RELEASE)` unless you explicitly set `fullnameOverride` that way.

`make helm-status` already selects ingress resources using `app.kubernetes.io/instance=$(HELM_RELEASE)` labels, so it remains accurate regardless of default naming or `fullnameOverride`.

If `TLS_SECRET` is not present yet (for example, cert-manager has not issued it yet, or TLS is intentionally disabled), `make helm-status` prints a non-fatal note instead of failing the whole check.

---

## 6) Operational notes

- Rotate webhook and app credentials by updating `github-app-secrets` and restarting pods if needed.
- Rotate TLS cert/key by updating the TLS secret referenced by `ingress.tls.secretName`.
- Keep `ingress.host` aligned with your GitHub App webhook URL host.
- For first-time public setup, use staging ACME issuer first, then switch to production after validation.

---

## 7) Operating CI deployment and troubleshooting

This repository ships with a GitHub Actions workflow at `.github/workflows/ci.yml` that runs:

- `test` job: dependency verification, formatting check, `go vet`, `golangci-lint`, race-enabled unit tests with a coverage artifact, and binary build.
- `integration` job: Kind-based deploy/verify flow via `make kind-deploy-verify` (Docker + Kind + Helm).

### 7.1 Deploy (enable) CI in your repository

1. Push the workflow file to your default branch (typically `main`).
2. Confirm GitHub Actions is enabled for the repository under **Settings > Actions**.
3. Open a pull request to verify the `CI / test` and `CI / integration` checks appear.
4. Optionally require both checks in branch protection rules before merge.

### 7.2 What the integration job deploys

The `integration` job performs an ephemeral cluster deployment on the Actions runner:

1. Creates a Kind cluster named `github-app`.
2. Installs ingress-nginx and cert-manager (via Makefile targets used by `make kind-deploy-verify`).
3. Builds Docker image `github-app:dev` and loads it into Kind.
4. Deploys the Helm release via `make kind-deploy-local`.
5. Port-forwards the service and verifies `/healthz` via `scripts/kind-deploy-verify.sh`.
6. Runs cleanup (`make kind-clean`) even when earlier steps fail.

This deployment is temporary and exists only for the duration of the workflow run.

### 7.3 CI troubleshooting playbook

Use the failed job logs in GitHub Actions first, then map to likely causes:

- **Formatting failure (`Verify formatting`)**
  - Symptom: workflow reports files that are not gofmt-formatted.
  - Fix: run `gofmt -w` on the listed `.go` files, commit, and rerun.

- **`go vet` or unit test failures in `test` job**
  - Symptom: failures before integration starts.
  - Fix: reproduce locally with `go vet ./...` and `go test -race -count=1 ./...`, then push fixes.

- **Kind bootstrap or Kubernetes rollout timeout**
  - Symptom: failure in `make kind-deploy-verify` while waiting for ingress-nginx/cert-manager/deployment.
  - Fixes:
    - Inspect logs around `make kind-bootstrap` and rollout status lines.
    - Rerun the failed job (transient runner/network pull issues are common).
    - If persistent, increase timeout-related Make variables (`KIND_INGRESS_WAIT_TIMEOUT`, `KIND_CERT_MANAGER_WAIT_TIMEOUT`) in the CI step.

- **Docker build/load failure**
  - Symptom: failure during `make docker-build` or `kind load docker-image`.
  - Fix: verify Dockerfile/build context changes and image tag assumptions (`github-app:dev`).

- **Helm deployment or health-check failure**
  - Symptom: release upgrades but `/healthz` probe fails.
  - Fixes:
    - Check rollout and service discovery logs emitted by `scripts/kind-deploy-verify.sh`.
    - Review `/tmp/github-app-port-forward.log` output included by script hints.
    - Validate chart values and secret wiring used by `kind-deploy-local`.

- **Cleanup failures (`make kind-clean`)**
  - Symptom: non-blocking cleanup noise after root-cause failure.
  - Fix: focus on first failing step; cleanup often reports follow-on errors when cluster/release was never fully created.

### 7.4 Safe rerun guidance

- Prefer rerunning only failed jobs when available.
- If failures are deterministic, reproduce with local Kind flow:

```bash
make kind-deploy-verify
```

- After local reproduction, apply fixes and push; GitHub Actions will redeploy cleanly in a new ephemeral runner environment.

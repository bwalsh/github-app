# Deploying `github-app` on Kind with ingress-nginx and cert-manager

This guide bootstraps a local Kind cluster that closely mirrors production Kubernetes deployment patterns.

## Prerequisites

- Docker
- `kind`
- `kubectl`
- `helm`

## 1) Bootstrap a Kind cluster with ingress-nginx and cert-manager

```bash
make kind-bootstrap
```

This target:

1. Creates a Kind cluster (`github-app`) if it does not exist.
2. Installs ingress-nginx.
3. Installs cert-manager and waits for the deployment to become available.

## 2) Install issuer manifests

Install the Let’s Encrypt issuers plus the local fallback issuer:

```bash
make kind-install-issuers
```

Installed manifests:

- `deploy/issuers/letsencrypt-staging.yaml`
- `deploy/issuers/letsencrypt-production.yaml`
- `deploy/issuers/selfsigned-local.yaml`

## 3) Create Kubernetes secrets for GitHub App credentials

The chart reads app credentials and webhook secrets via Kubernetes Secret references. Use:

```bash
export GITHUB_WEBHOOK_SECRET='replace-me'
export GITHUB_APP_ID='123456'
export GITHUB_APP_INSTALLATION_ID='654321'
export GITHUB_APP_PRIVATE_KEY_FILE='/path/to/private-key.pem'

make kind-create-secrets
```

This creates/updates the `github-app-secrets` Secret in namespace `github-app`.

`kind-create-secrets` also accepts `GITHUB_APP_PRIVATE_KEY` directly for compatibility, but
`GITHUB_APP_PRIVATE_KEY_FILE` is recommended to avoid brittle multiline shell handling.

### Credential precedence at runtime

When the app needs to report checks or commit statuses, it resolves GitHub API credentials in this order:

1. `GITHUB_APP_ID` + `GITHUB_APP_PRIVATE_KEY`
   - Preferred mode.
   - The service mints installation tokens dynamically per job using the webhook `installation_id`.
2. `GITHUB_TOKEN`
   - Fallback mode.
   - Reuses one token for every job, so treat it as a single-installation compatibility option.
3. No GitHub API credentials
   - The service still starts, but GitHub reporting uses the mock client.

`GITHUB_APP_INSTALLATION_ID` may still be present in the secret because the helper target and chart support it, but dynamic GitHub App credentials take precedence whenever `GITHUB_APP_ID` and `GITHUB_APP_PRIVATE_KEY` are both configured.

If you need the static `GITHUB_TOKEN` fallback in Kind, inject it explicitly through Helm `extraEnv`; the stock chart only wires the webhook secret plus GitHub App credential keys.

### Secret schema consumed by the chart

- `github-webhook-secret` -> `GITHUB_WEBHOOK_SECRET`
- `github-app-id` -> `GITHUB_APP_ID`
- `github-app-installation-id` -> `GITHUB_APP_INSTALLATION_ID`
- `github-app-private-key` -> `GITHUB_APP_PRIVATE_KEY_FILE` (recommended) or `GITHUB_APP_PRIVATE_KEY`

> Do not commit secret values, PEM files, or rendered Secret manifests to Git.

## 4) Deploy the Helm chart

Before Helm deploy, the `kind-deploy-*` targets build a local image and load it into the Kind cluster.
By default this image is tagged `github-app:dev`. You can override with `IMG_REPO` and `IMG_TAG`.

```bash
make kind-load-image IMG_REPO=github-app IMG_TAG=dev
```

If you run deploy directly, image build/load is automatic because deploy targets depend on `kind-load-image`.

For publicly reachable environments (staging/prod-like), use Let’s Encrypt staging first:

```bash
make kind-deploy-staging HOST=webhooks.example.com TLS_SECRET=github-app-webhooks-tls
```

For local-only environments (no public DNS / no public HTTP-01 validation), switch to the local fallback issuer:

```bash
make kind-deploy-local HOST=github-app.localdev.me TLS_SECRET=github-app-local-tls
```

## 5) Validate chart rendering and linting

```bash
make helm-validate
```

## 6) Cleanup

```bash
make kind-clean
```

## Local fallback issuer strategy (non-public environments)

Let’s Encrypt HTTP-01 requires a publicly reachable hostname. For disconnected or laptop-only development,
use the `selfsigned-local` ClusterIssuer and local DNS-like hostnames (for example `*.localdev.me`).

Strategy:

1. Keep ingress + TLS enabled to exercise the same Kubernetes objects as production.
2. Set `certManager.localFallbackIssuer.enabled=true`.
3. Use the `selfsigned-local` ClusterIssuer.
4. Trust the generated cert only for local testing when needed.

This lets teams validate chart behavior, probe wiring, ingress rules, and cert-manager integration before
moving to a public endpoint with Let’s Encrypt.

# Operator guide: installing and operating the `github-app` Helm chart

This guide is for cluster operators deploying `charts/github-app` into Kubernetes environments outside the local Kind bootstrap flow.

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

---

## 6) Operational notes

- Rotate webhook and app credentials by updating `github-app-secrets` and restarting pods if needed.
- Rotate TLS cert/key by updating the TLS secret referenced by `ingress.tls.secretName`.
- Keep `ingress.host` aligned with your GitHub App webhook URL host.
- For first-time public setup, use staging ACME issuer first, then switch to production after validation.

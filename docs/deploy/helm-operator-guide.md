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
kubectl create namespace github-app --dry-run=client -o yaml | kubectl apply -f -
```

Create/update the secret consumed by the chart:

```bash
kubectl -n github-app create secret generic github-app-secrets \
  --from-literal=github-webhook-secret='replace-me' \
  --from-literal=github-app-id='123456' \
  --from-literal=github-app-installation-id='654321' \
  --from-file=github-app-private-key=/path/to/github-app.private-key.pem \
  --dry-run=client -o yaml | kubectl apply -f -
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
helm upgrade --install github-app ./charts/github-app \
  --namespace github-app \
  --set image.repository=ghcr.io/<org>/<image> \
  --set image.tag=<tag> \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.host=webhooks.example.com \
  --set ingress.tls.enabled=true \
  --set ingress.tls.secretName=github-app-webhooks-tls \
  --set certManager.enabled=true \
  --set certManager.clusterIssuer.name=letsencrypt-production
```

Recommended: commit environment-specific values files (without secrets) such as:

- `values-staging.yaml`
- `values-production.yaml`

Then install with:

```bash
helm upgrade --install github-app ./charts/github-app \
  --namespace github-app \
  -f values-production.yaml
```

---

## 4) Supplying your own ingress public/private key pair

If your organization manages TLS certificates outside cert-manager, provide your own certificate and private key as a TLS secret and configure ingress to use it.

### 4.1 Create TLS secret from your cert/key

```bash
kubectl -n github-app create secret tls github-app-webhooks-tls \
  --cert=/path/to/tls.crt \
  --key=/path/to/tls.key \
  --dry-run=client -o yaml | kubectl apply -f -
```

- `tls.crt` is your public certificate chain.
- `tls.key` is your private key.

### 4.2 Disable cert-manager integration and reference the secret

Use Helm overrides:

```bash
helm upgrade --install github-app ./charts/github-app \
  --namespace github-app \
  --set ingress.enabled=true \
  --set ingress.host=webhooks.example.com \
  --set ingress.tls.enabled=true \
  --set ingress.tls.secretName=github-app-webhooks-tls \
  --set certManager.enabled=false
```

With `certManager.enabled=false`, the chart will not set `cert-manager.io/cluster-issuer` on the Ingress and your pre-created TLS secret will be used directly.

---

## 5) Post-install checks

```bash
kubectl -n github-app get deploy,pods,svc,ingress
kubectl -n github-app describe ingress github-app
kubectl -n github-app get secret github-app-webhooks-tls
helm -n github-app status github-app
```

Validate service health:

- In-cluster probe path is `/healthz`.
- GitHub webhook endpoint path is `/webhook`.

---

## 6) Operational notes

- Rotate webhook and app credentials by updating `github-app-secrets` and restarting pods if needed.
- Rotate TLS cert/key by updating the TLS secret referenced by `ingress.tls.secretName`.
- Keep `ingress.host` aligned with your GitHub App webhook URL host.
- For first-time public setup, use staging ACME issuer first, then switch to production after validation.

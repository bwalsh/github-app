# Exposing the webhook endpoint to GitHub

GitHub must be able to reach your webhook URL over public HTTPS.

## Required endpoint contract

- Webhook URL path: `/webhook`
- Health endpoint: `/healthz`
- HTTPS with a certificate trusted by GitHub

## Option A: Public DNS + ingress (recommended)

1. Point a public DNS A/AAAA/CNAME record to your ingress endpoint.
2. Deploy with `letsencrypt-staging` first.
3. Verify webhook delivery from GitHub.
4. Switch to `letsencrypt-production` after validation.

Example Helm settings:

```yaml
ingress:
  host: webhooks.example.com
  tls:
    enabled: true
    secretName: github-app-webhooks-tls
certManager:
  enabled: true
  clusterIssuer:
    name: letsencrypt-production
  localFallbackIssuer:
    enabled: false
```

## Option B: Tunnel to local Kind ingress

For local iteration, use a secure tunnel provider to expose ingress temporarily. The provider must terminate
or pass through TLS in a way GitHub accepts.

High-level flow:

1. Start tunnel to ingress-nginx service or node port.
2. Use tunnel hostname as `ingress.host`.
3. Configure GitHub App webhook URL to `https://<tunnel-host>/webhook`.
4. Reconcile cert strategy:
   - If the host is publicly valid and HTTP-01 is reachable, use Let’s Encrypt.
   - Otherwise use `selfsigned-local` for local tests that do not require GitHub delivery.

## GitHub App webhook configuration

In GitHub App settings:

1. Set **Webhook URL** to `https://<public-host>/webhook`.
2. Set **Webhook secret** to match the value stored in `github-app-secrets[github-webhook-secret]`.
3. Subscribe to required events.
4. Trigger a test delivery and inspect app logs.

## Verification checklist

- `kubectl -n github-app get ingress`
- `kubectl -n github-app get certificate,secret`
- `kubectl -n github-app get pods`
- Confirm readiness/liveness probes are passing on `/healthz`.
- Confirm GitHub webhook deliveries return 2xx.

# API Endpoints

This document describes the HTTP endpoints exposed by `github-app` and the current runtime behavior implemented in:

- `cmd/github-app/main.go` (`buildMux`)
- `internal/handler/handler.go` (`HandleWebhook`)

## Base URL

Use your deployed host and port, for example:

- Local: `http://localhost:8080`
- Public webhook: `https://<your-host>`

## Endpoint Summary

| Endpoint | Method | Purpose |
|---|---|---|
| `/webhook` | `POST` | Receives GitHub webhook events and queues tenant-scoped work |
| `/healthz` | `GET` | Health check endpoint |

## `POST /webhook`

Receives GitHub App webhook payloads.

### Required Request Headers

| Header | Required | Notes |
|---|---|---|
| `X-GitHub-Event` | Yes | Event type, such as `push` or `installation_repositories` |
| `X-Hub-Signature-256` | Yes | Signature in format `sha256=<hex>` |
| `Content-Type` | Recommended | `application/json` |

### Signature Validation

The webhook body is authenticated using HMAC-SHA256 and `GITHUB_WEBHOOK_SECRET`.

- Signature format: `sha256=<hex(hmac_sha256(raw_request_body, secret))>`
- If missing or invalid, endpoint returns `401 invalid signature`

### Status Codes

| Status | When |
|---|---|
| `200 OK` | Event accepted/handled/ignored normally |
| `400 Bad Request` | Missing `X-GitHub-Event` or invalid JSON payload |
| `401 Unauthorized` | Missing or invalid `X-Hub-Signature-256` |
| `409 Conflict` | `installation_repositories` tries to onboard an already-registered repo |
| `500 Internal Server Error` | Tenant registry lookup/register failure |
| `503 Service Unavailable` | Queue is full |

### Event Behavior

#### `X-GitHub-Event: push`

- Only `ref == refs/heads/main` triggers deployment queueing.
- Non-main refs are accepted but ignored (returns `200`).
- If tenant mapping exists:
  - Enqueues `push_deploy` job.
  - Returns `200 accepted push`.
- If no tenant mapping exists:
  - Returns `200 accepted push event ... (no tenant mapping)`.

#### `X-GitHub-Event: installation_repositories`

- Only `action == added` is processed.
- For each repo in `repositories_added`:
  - Registers tenant mapping `(installation_id, repository_id) -> tenant`.
  - Enqueues `repo_onboarding` job.
- Non-`added` actions are accepted and ignored with `200`.
- Duplicate onboarding for an existing mapping returns `409`.

#### Other Events

- Parsed as generic payloads.
- Logged and acknowledged with `200`.

### Example: Valid Signed Request (local test)

```bash
body='{"action":"opened","repository":{"full_name":"org/repo"},"sender":{"login":"alice"}}'
secret='replace-me'
sig=$(printf '%s' "$body" | openssl dgst -sha256 -hmac "$secret" -hex | sed 's/^.* //')

curl -i -X POST http://localhost:8080/webhook \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: pull_request" \
  -H "X-Hub-Signature-256: sha256=$sig" \
  -d "$body"
```

## `GET /healthz`

Simple health endpoint.

### Status Codes

| Status | Body |
|---|---|
| `200 OK` | `ok` |

### Example

```bash
curl -i http://localhost:8080/healthz
```

## Operational Notes

- Queue-backed async processing means webhook acceptance (`200`) indicates receipt and enqueue attempt, not full workflow completion.
- Commit status/check-run updates are performed by worker jobs after enqueue.
- For webhook setup guidance, see `docs/deploy/github-webhook-exposure.md`.


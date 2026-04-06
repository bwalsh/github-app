# github-app

[![CI](https://github.com/bwalsh/github-app/actions/workflows/ci.yml/badge.svg)](https://github.com/bwalsh/github-app/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/bwalsh/github-app)](https://github.com/bwalsh/github-app/releases)
![Coverage](https://img.shields.io/endpoint?label=coverage&url=https://raw.githubusercontent.com/bwalsh/github-app/refs/heads/main/.github/badges/coverage.json)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A GitHub App for webhook event registration and handling, written in Go.

## Features

- Receives and processes GitHub App webhook events
- Health check endpoint (`/healthz`)
- Configurable via environment variables
- Pluggable tenant persistence (`memory` or `sqlite`)

## Requirements

- Go 1.24.13 or later

## Installation

```bash
git clone https://github.com/bwalsh/github-app.git
cd github-app
make build
```

The compiled binary will be placed in `bin/github-app`.

## Usage

Set the required environment variables and run the binary:

```bash
export GITHUB_WEBHOOK_SECRET=your-secret
export PORT=8080          # optional, defaults to 8080
export TENANT_PERSISTENCE=memory  # optional: memory (default) or sqlite
export TENANT_SQLITE_DSN=/tmp/tenants.db # optional when using sqlite (default: /tmp/tenants.db)

./bin/github-app
```

### GitHub API credentials

When the service needs to create check runs or commit statuses, it resolves credentials in this order:

1. `GITHUB_APP_ID` **and** `GITHUB_APP_PRIVATE_KEY`
   - Preferred mode.
   - The app mints installation tokens dynamically per webhook job, keyed by `installation_id`.
   - Use this for multi-installation deployments.
2. `GITHUB_TOKEN`
   - Fallback mode.
   - The same token is reused for every job, so this is only appropriate when you intentionally operate against a single installation or repository scope.
3. No GitHub API credentials
   - The service falls back to the mock GitHub client and does not call the real GitHub API.

Notes:

- `GITHUB_APP_ID` and `GITHUB_APP_PRIVATE_KEY` must be set together.
- `GITHUB_APP_PRIVATE_KEY` may be provided either as a normal PEM block or with escaped `\n` newlines.
- `GITHUB_APP_INSTALLATION_ID` may still appear in Kubernetes secret plumbing for compatibility, but runtime credential selection prefers dynamic installation tokens whenever `GITHUB_APP_ID` and `GITHUB_APP_PRIVATE_KEY` are present.

### Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/webhook` | `POST` | Receives GitHub webhook events (requires valid `X-Hub-Signature-256`) |
| `/healthz` | `GET` | Health check |

Full endpoint contract, status codes, and request examples: [`docs/api/endpoints.md`](docs/api/endpoints.md)

## Development

All development tasks are driven by the `Makefile`. Run `make help` for a full list of targets.

```bash
make build     # Compile the binary
make test      # Run tests with race detection
make test-unit # Run unit tests only
make test-integration # Run integration tests only
make test-signature # Run webhook signature validation tests
make test-tenant # Run tenant persistence tests (memory + sqlite)
make test-tenant-sqlite # Run only sqlite persistence tests
make coverage  # Generate HTML coverage report (coverage.html)
make lint      # Run go vet
make release   # Build release binaries for Linux, macOS, and Windows
make clean     # Remove build artifacts
```

## Kubernetes Deployment (Kind + Helm + cert-manager)

This repository includes a Helm chart at `charts/github-app` and bootstrap automation for local Kind clusters.
The Kind deploy targets build `github-app:dev` locally and load it into the Kind cluster before running Helm.

- Bootstrap guide: [`docs/deploy/kind-helm-cert-manager.md`](docs/deploy/kind-helm-cert-manager.md)
- Webhook exposure guide: [`docs/deploy/github-webhook-exposure.md`](docs/deploy/github-webhook-exposure.md)
- Operator guide: [`docs/deploy/helm-operator-guide.md`](docs/deploy/helm-operator-guide.md)
- Tenant persistence architecture and operations: [`docs/architecture/persistence-layer.md`](docs/architecture/persistence-layer.md)
- GitHub integration flows: [`docs/flows/github-integration-flows.md`](docs/flows/github-integration-flows.md)
- API endpoint reference: [`docs/api/endpoints.md`](docs/api/endpoints.md)
- One-shot local bootstrap/deploy/verify script: `scripts/kind-deploy-verify.sh`
- Issuer manifests:
  - `deploy/issuers/letsencrypt-staging.yaml`
  - `deploy/issuers/letsencrypt-production.yaml`
  - `deploy/issuers/selfsigned-local.yaml`

Quick start:

```bash
export GITHUB_WEBHOOK_SECRET=replace-me
./scripts/kind-deploy-verify.sh
```

For Kubernetes/operator deployments, prefer supplying `GITHUB_APP_ID` plus `GITHUB_APP_PRIVATE_KEY` so the app can mint installation-specific tokens. Use `GITHUB_TOKEN` only as a static single-installation fallback; see [`docs/deploy/helm-operator-guide.md`](docs/deploy/helm-operator-guide.md) for the credential wiring options.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

This project is licensed under the [MIT License](LICENSE).

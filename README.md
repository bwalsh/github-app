# github-app

A GitHub App for webhook event registration and handling, written in Go.

## Features

- Receives and processes GitHub App webhook events
- Health check endpoint (`/healthz`)
- Configurable via environment variables

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

./bin/github-app
```

### Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/webhook` | `POST` | Receives GitHub webhook events |
| `/healthz` | `GET` | Health check |

## Development

All development tasks are driven by the `Makefile`. Run `make help` for a full list of targets.

```bash
make build     # Compile the binary
make test      # Run tests with race detection
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

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

This project is licensed under the [MIT License](LICENSE).

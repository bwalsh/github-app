# Contributing to github-app

Thank you for your interest in contributing! The following guidelines will help you get started.

## Getting Started

1. Fork the repository and clone it locally.
2. Ensure you have Go 1.21 or later installed.
3. Install dependencies and verify the build:

```bash
make build
make test
```

## Development Workflow

All common tasks are driven by the `Makefile`. Run `make help` to see all available targets.

| Command | Description |
|---------|-------------|
| `make build` | Compile the binary |
| `make test` | Run all tests with race detection |
| `make coverage` | Run tests and generate an HTML coverage report |
| `make lint` | Run `go vet` |
| `make release` | Build release binaries for multiple platforms |
| `make clean` | Remove build artifacts |

## Code Style

- Follow standard Go conventions ([Effective Go](https://go.dev/doc/effective_go)).
- Run `go fmt ./...` before committing.
- Ensure `go vet ./...` passes without warnings.
- All exported types, functions, and methods must have doc comments.

## Submitting Changes

1. Create a feature branch from `main`: `git checkout -b feature/my-change`.
2. Make your changes with appropriate tests.
3. Ensure `make test` and `make lint` pass.
4. Open a pull request against `main` with a clear description of your changes.

## Reporting Issues

Please open an issue on GitHub with a description of the problem, steps to reproduce, and the expected vs. actual behavior.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).

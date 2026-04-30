# Development

## Common commands

```bash
# Run the controller locally against your current kubeconfig
make run

# Build the operator
make build

# Build the CLI
make build-cli

# Run tests
make test

# Run lint
make lint

# Run golangci-lint fixes
make lint-fix

# Build an operator image
make docker-build IMG=ghcr.io/slok-operator/slok:dev
```

## CLI development

Run the CLI directly:

```bash
make run-cli ARGS="backtest --help"
```

Run CLI-related tests:

```bash
go test ./cmd/slok ./internal/backtest
```

## Test suite

The main test target runs:

- manifest generation
- deepcopy generation
- `go fmt ./...`
- `go vet ./...`
- Go tests, excluding e2e

```bash
make test
```

## Lint

```bash
make lint
```

This uses `golangci-lint` with the repository configuration.

## Generate manifests

```bash
make manifests
make generate
```

## Build and deploy a local image

```bash
make docker-build IMG=ghcr.io/slok-operator/slok:dev
make deploy IMG=ghcr.io/slok-operator/slok:dev
```

## E2E tests

The repository contains e2e test scaffolding based on Kind:

```bash
make test-e2e
```

Use this only when you want an isolated Kubernetes test environment.

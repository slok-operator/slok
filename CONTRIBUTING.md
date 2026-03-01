# Contributing to SLOK

Thank you for your interest in contributing! This document covers how to set up a development environment, run the test suite, and submit changes.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Running Tests](#running-tests)
- [Submitting Changes](#submitting-changes)
- [Coding Guidelines](#coding-guidelines)
- [Reporting Issues](#reporting-issues)

## Code of Conduct

Be respectful and constructive. We follow the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/main/code-of-conduct.md).

## Getting Started

1. **Fork** the repository and clone your fork
2. Create a **feature branch** from `main`: `git checkout -b feat/my-feature`
3. Make your changes, write tests, and open a pull request

For bug fixes, open an issue first so we can discuss the approach before you invest time writing code.

## Development Setup

### Prerequisites

| Tool | Version | Install |
|------|---------|---------|
| Go | 1.23+ | https://go.dev/dl |
| Docker | 24+ | https://docs.docker.com/get-docker |
| kubectl | 1.20+ | https://kubernetes.io/docs/tasks/tools |
| kubebuilder | 4+ | https://book.kubebuilder.io/quick-start |
| golangci-lint | 2.4+ | https://golangci-lint.run/welcome/install |
| Helm | 3+ | https://helm.sh/docs/intro/install |

### Clone and build

```bash
git clone https://github.com/federicolepera/slok.git
cd slok
go build ./...
```

### Run locally against a cluster

```bash
# Point KUBECONFIG at your cluster, then:
make install   # install CRDs
make run       # run the operator locally
```

Set `PROMETHEUS_URL` to point at your Prometheus instance:

```bash
export PROMETHEUS_URL=http://localhost:9090
make run
```

## Running Tests

```bash
# Unit + integration tests (uses envtest, no real cluster needed)
make test

# Lint
golangci-lint run

# Build the Docker image
make docker-build IMG=your-registry/slok:dev
```

The test suite uses [Ginkgo](https://onsi.github.io/ginkgo/) and [envtest](https://book.kubebuilder.io/reference/envtest). No external dependencies are required.

## Submitting Changes

### Commit messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add WEIGHTED_ROUTES composition strategy
fix: correct error budget calculation for windows > 30d
docs: update SLOComposition examples in README
chore: upgrade controller-runtime to v0.22
```

### Pull request checklist

- [ ] `go build ./...` passes
- [ ] `make test` passes
- [ ] `golangci-lint run` passes with no new warnings
- [ ] New behaviour is covered by tests
- [ ] README or docs updated if the user-facing API changed
- [ ] CRDs regenerated if types changed: `make generate && make manifests`

### CRD and RBAC regeneration

If you modify files under `api/`, regenerate the manifests before opening your PR:

```bash
make generate   # regenerate DeepCopy methods
make manifests  # regenerate CRDs and RBAC
```

The generated files under `config/crd/bases/` and `config/rbac/` must be committed alongside the type changes. Also copy the updated CRDs to `charts/slok/crds/` to keep the Helm chart in sync.

## Coding Guidelines

- **No scaffolding leftovers**: remove all `TODO(user):` comments before submitting.
- **Tests first for controllers**: use [envtest](https://book.kubebuilder.io/reference/envtest) integration tests, not mocks only.
- **Avoid breaking API changes**: the CRD API is versioned `v1alpha1`; additive changes are preferred. Breaking changes require a discussion in an issue first.
- **Keep controllers idempotent**: reconcile loops must produce the same result when called multiple times with the same inputs.
- **Prometheus queries**: do not include `sum()`, `rate()`, or `increase()` in user-provided queries — the operator adds these automatically.

## Reporting Issues

Open a [GitHub issue](https://github.com/federicolepera/slok/issues/new) and include:

- SLOK version (image tag or commit)
- Kubernetes version (`kubectl version`)
- Prometheus version
- The `ServiceLevelObjective` or `SLOComposition` YAML (redact sensitive labels)
- Relevant operator logs (`kubectl logs -n slok-system deploy/slok-controller-manager`)

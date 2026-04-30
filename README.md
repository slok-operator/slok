# SloK

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.20%2B-brightgreen.svg)](https://kubernetes.io)
[![Go Report Card](https://goreportcard.com/badge/github.com/slok-operator/slok)](https://goreportcard.com/report/github.com/slok-operator/slok)

SloK is a Kubernetes-native SLO operator.

It lets you define Service Level Objectives as Kubernetes resources, generates the
Prometheus recording and alerting rules needed to monitor them, tracks error budget
and burn rate, and provides a CLI to backtest SLOs against historical Prometheus data
before applying them to the cluster.

## What SloK does

- Defines SLOs as Kubernetes custom resources.
- Generates Prometheus recording rules for SLI error rate, error budget, and burn rate.
- Generates burn-rate and error-budget alerts using `PrometheusRule` resources.
- Tracks current objective status in the Kubernetes resource status.
- Supports built-in SLI templates and manual PromQL metric selectors.
- Backtests SLO YAML files before they are applied to the cluster.
- Supports SLO composition for higher-level user journeys.
- Correlates SLO degradation with recent Kubernetes changes.
- Ships with an optional web dashboard for browsing SLOs and historical trends.

## Quick start

Install SloK with Helm:

```bash
helm install slok charts/slok \
  --namespace slok-system \
  --create-namespace \
  --set prometheus.url=http://prometheus-kube-prometheus-prometheus.monitoring.svc:9090
```

Create an SLO:

```yaml
apiVersion: observability.slok.io/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: checkout-availability
  namespace: default
spec:
  displayName: Checkout Availability
  objective:
    name: availability
    target: 99.9
    window: 30d
    sli:
      query:
        totalQuery: http_requests_total{service="checkout"}
        errorQuery: http_requests_total{service="checkout",status=~"5.."}
```

Apply it:

```bash
kubectl apply -f slo.yaml
```

Check status:

```bash
kubectl get slo
kubectl get slo checkout-availability -o yaml
```

## Backtest SLOs before applying them

SloK includes a CLI for testing an SLO against historical Prometheus data.

Build the CLI locally:

```bash
make build-cli
```

Backtest an SLO that has already been applied to the cluster:

```bash
bin/slok backtest \
  --namespace default \
  --name checkout-availability \
  --prometheus-url http://localhost:9090
```

This mode uses existing SloK recording rules in Prometheus.

You can also backtest an SLO YAML before applying it:

```bash
bin/slok backtest \
  -f slo.yaml \
  --pre-apply \
  --prometheus-url http://localhost:9090
```

Pre-apply backtesting reads:

```yaml
spec:
  objective:
    sli:
      query:
        totalQuery: ...
        errorQuery: ...
```

and queries Prometheus directly, so the SLO does not need to exist in the cluster yet.

You can compare multiple target values with what-if mode:

```bash
bin/slok backtest \
  -f slo.yaml \
  --pre-apply \
  --targets 99,99.5,99.9,99.95 \
  --prometheus-url http://localhost:9090
```

Current limitation: pre-apply backtesting supports manual `totalQuery` / `errorQuery`
SLIs. Template-based pre-apply backtesting is planned.

## Dashboard

SloK can deploy an optional dashboard alongside the operator.

The dashboard lists SLOs, shows objective status and error budget, and provides
historical trend charts backed by Prometheus.

Enable it through the Helm chart:

```bash
helm install slok charts/slok \
  --namespace slok-system \
  --create-namespace \
  --set dashboard.enabled=true \
  --set dashboard.ingress.enabled=true \
  --set dashboard.ingress.className=nginx \
  --set dashboard.ingress.hosts[0].host=slok-dashboard.local \
  --set dashboard.prometheus.url=http://prometheus-kube-prometheus-prometheus.monitoring.svc:9090
```

The dashboard is intentionally authentication-free. Put it behind your existing
Ingress authentication, VPN, oauth2-proxy, or internal network controls.

![SloK Dashboard](docs/images/dashboard.png)

## SLO definitions

An SLO contains one objective. The objective defines the target, the rolling window,
and the SLI used to measure reliability.

### Manual queries

Manual SLIs use two Prometheus metric selectors:

- `totalQuery`: total events
- `errorQuery`: failed events

```yaml
apiVersion: observability.slok.io/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: payment-api-availability
spec:
  displayName: Payment API Availability
  objective:
    name: availability
    target: 99.9
    window: 30d
    sli:
      query:
        totalQuery: http_requests_total{service="payment-api"}
        errorQuery: http_requests_total{service="payment-api",status=~"5.."}
```

Queries should be metric selectors only. Do not include `sum()`, `rate()`, or
`increase()`. SloK wraps the selectors when generating recording rules.

### Templates

Templates avoid writing raw PromQL for common SLI patterns.

```yaml
apiVersion: observability.slok.io/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: payment-api-availability
spec:
  displayName: Payment API Availability
  objective:
    name: availability
    target: 99.9
    window: 30d
    sli:
      template:
        name: http-availability
        labels:
          service: payment-api
```

Available templates:

| Template | Description | Required params |
|----------|-------------|-----------------|
| `http-availability` | HTTP request success rate, based on non-5xx responses | none |
| `http-latency` | HTTP latency SLO based on histogram buckets | `threshold` |
| `kubernetes-apiserver` | Kubernetes API server availability | none |

## Alerting and recording rules

For each SLO, SloK generates Prometheus recording rules similar to:

```text
slok:sli_error_rate:5m
slok:sli_error_rate:1h
slok:sli_error_rate:6h
slok:sli_error_rate:3d
slok:sli_error_rate:7d
slok:sli_error_rate:30d
slok:error_budget_target
slok:burn_rate:5m
slok:burn_rate:1h
...
```

Burn-rate alerts follow the multi-window, multi-burn-rate approach from the
[Google SRE Workbook](https://sre.google/workbook/alerting-on-slos/).

When enabled, SloK can create default alerts for:

- fast burn / active outage
- medium burn / degraded behavior
- slow burn / steady erosion
- exhausted error budget

Custom error-budget and burn-rate alerts can also be configured per SLO.

## SLO composition

SloK can combine multiple SLOs into a higher-level objective with `SLOComposition`.

This is useful for user journeys where reliability depends on several services.

Supported composition strategies:

| Strategy | Description | Status |
|----------|-------------|--------|
| `AND_MIN` | The composition is only as healthy as the weakest referenced SLO | Stable |
| `WEIGHTED_ROUTES` | Models traffic flowing through weighted service chains | Alpha |

Example:

```yaml
apiVersion: observability.slok.io/v1alpha1
kind: SLOComposition
metadata:
  name: checkout-flow
spec:
  target: 99.9
  window: 30d
  objectives:
    - name: frontend
      ref:
        name: frontend-availability
    - name: payments
      ref:
        name: payments-availability
  composition:
    type: AND_MIN
```

## Event correlation

SloK can correlate burn-rate spikes with recent Kubernetes changes such as
Deployment updates, ConfigMap changes, Secret changes, and relevant Events.

When a spike is detected, it creates an `SLOCorrelation` resource with candidate
causes and confidence levels.

```bash
kubectl get slocorr
kubectl get slocorr <name> -o yaml
```

An optional Groq integration can refine correlation summaries when `GROQ_API_KEY`
is configured. Without it, SloK uses its built-in rule-based summary.

## Installation options

### Helm

```bash
helm install slok charts/slok \
  --namespace slok-system \
  --create-namespace
```

Useful values:

| Value | Description | Default |
|-------|-------------|---------|
| `image.repository` | Operator image | `ghcr.io/slok-operator/slok` |
| `prometheus.url` | Prometheus URL used by the operator | empty |
| `webhook.enabled` | Enable admission webhooks | `true` |
| `certManager.enabled` | Use cert-manager for webhook certificates | `true` |
| `dashboard.enabled` | Deploy the optional dashboard | `true` |
| `dashboard.ingress.enabled` | Create dashboard Ingress | `true` |
| `dashboard.prometheus.url` | Prometheus URL used by dashboard charts | kube-prometheus-stack default |

### Kustomize

For local development:

```bash
kubectl apply -k config/default
```

## Requirements

| Requirement | Notes |
|-------------|-------|
| Kubernetes 1.20+ | CRDs and controller runtime |
| Prometheus 2.x+ | Required for SLI, burn-rate, and backtest queries |
| Prometheus Operator | Required for generated `PrometheusRule` resources |
| cert-manager | Required when webhooks are enabled |

## Development

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

# Build an operator image
make docker-build IMG=ghcr.io/slok-operator/slok:dev
```

## Current limitations

- Pre-apply backtesting currently supports manual `totalQuery` / `errorQuery` SLIs,
  not template-based SLIs.
- `WEIGHTED_ROUTES` composition is alpha and may change.
- One operator instance is expected per cluster.
- The dashboard does not implement authentication itself.

## License

Apache License 2.0. See [LICENSE](LICENSE) for details.

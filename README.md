# SLOK - Service Level Objectives for Kubernetes

SLOK is a Kubernetes operator that manages Service Level Objectives (SLOs) with automatic error budget tracking. Define your reliability targets as Kubernetes resources, and SLOK will continuously monitor them using Prometheus.

## Quick Start

Get your first SLO running in 5 minutes:

```bash
# 1. Install the CRDs and operator
kubectl apply -k config/default

# 2. Create your first SLO
cat <<EOF | kubectl apply -f -
apiVersion: observability.slok.io/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: my-api-availability
spec:
  displayName: "My API Availability"
  objectives:
    - name: availability
      target: 99.9
      window: 7d
      sli:
        query: |
          (sum(rate(http_requests_total{status=~"2.."}[5m]))
           / sum(rate(http_requests_total[5m]))) * 100
EOF

# 3. Check the status
kubectl get slo my-api-availability -o yaml
```

## Prerequisites

| Requirement | Version | Notes |
|-------------|---------|-------|
| Kubernetes | 1.20+ | |
| Prometheus | 2.x+ | Must be accessible from the operator |
| Prometheus Operator | (optional) | Required for ServiceMonitor and PrometheusRule |
| cert-manager | 1.0+ | Required if using webhooks |

### Prometheus Setup

SLOK needs to query Prometheus for your SLI metrics. The operator connects to Prometheus via the `PROMETHEUS_URL` environment variable.

If you're using [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack), Prometheus is typically available at:
```
http://prometheus-kube-prometheus-prometheus.monitoring.svc:9090
```

## Installation

### Option 1: Kustomize (Quick)

```bash
# Install CRDs and deploy operator
kubectl apply -k config/default
```

### Option 2: Helm (Recommended for Production)

```bash
# Add the chart repository (if published) or install from local
helm install slok charts/slok \
  --namespace slok-system \
  --create-namespace \
  --set prometheus.url=http://prometheus-kube-prometheus-prometheus.monitoring.svc:9090
```

#### Helm Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `prometheus.url` | Prometheus server URL | `http://prometheus-kube-prometheus-prometheus.monitoring.svc:9090` |
| `webhook.enabled` | Enable admission webhooks | `true` |
| `metrics.enabled` | Enable metrics endpoint | `true` |
| `prometheusRule.enabled` | Deploy PrometheusRule for SLO alerts | `true` |
| `replicaCount` | Number of operator replicas | `1` |

Disable webhooks (useful for development):
```bash
helm install slok charts/slok \
  --set webhook.enabled=false \
  --set certManager.enabled=false
```

### Verify Installation

```bash
# Check operator is running
kubectl get pods -n slok-system

# Check CRD is installed
kubectl get crd servicelevelobjectives.observability.slok.io
```

## Examples

### Availability SLO

Track the percentage of successful HTTP requests:

```yaml
apiVersion: observability.slok.io/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: payment-api-availability
spec:
  displayName: "Payment API Availability"
  objectives:
    - name: availability
      target: 99.9        # Target: 99.9% successful requests
      window: 30d         # Over a 30-day rolling window
      sli:
        query: |
          (
            sum(rate(http_requests_total{service="payment-api", status=~"2.."}[5m]))
            /
            sum(rate(http_requests_total{service="payment-api"}[5m]))
          ) * 100
```

### Latency SLO

Track the percentage of requests under a latency threshold:

```yaml
apiVersion: observability.slok.io/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: checkout-latency
spec:
  displayName: "Checkout Latency"
  objectives:
    - name: p99-latency
      target: 95.0        # 95% of requests should be under threshold
      window: 7d
      sli:
        query: |
          (
            sum(rate(http_request_duration_seconds_bucket{service="checkout", le="0.5"}[5m]))
            /
            sum(rate(http_request_duration_seconds_count{service="checkout"}[5m]))
          ) * 100
```

### Multiple Objectives

Define multiple objectives in a single SLO:

```yaml
apiVersion: observability.slok.io/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: api-gateway-slo
spec:
  displayName: "API Gateway SLO"
  objectives:
    - name: availability
      target: 99.95
      window: 30d
      sli:
        query: |
          (sum(rate(http_requests_total{job="api-gateway", status!~"5.."}[5m]))
           / sum(rate(http_requests_total{job="api-gateway"}[5m]))) * 100

    - name: latency-p99
      target: 99.0
      window: 30d
      sli:
        query: |
          (sum(rate(http_request_duration_seconds_bucket{job="api-gateway", le="0.3"}[5m]))
           / sum(rate(http_request_duration_seconds_count{job="api-gateway"}[5m]))) * 100
```

### Check SLO Status

```bash
kubectl get slo payment-api-availability -o yaml
```

Output:
```yaml
status:
  objectives:
    - name: availability
      target: 99.9
      actual: 99.87
      status: met           # met | at-risk | violated
      errorBudget:
        total: "43.2m"
        consumed: "10.5m"
        remaining: "32.7m"
        percentRemaining: 75.69
      lastQueried: "2026-01-28T10:30:00Z"
  lastUpdateTime: "2026-01-28T10:30:00Z"
  conditions:
    - type: Available
      status: "True"
      reason: Reconciled
```

### Alerting with PrometheusRule

SLOK exposes metrics that you can use for alerting:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: slo-alerts
  labels:
    release: prometheus
spec:
  groups:
  - name: slo.alerts
    rules:
    - alert: SLOObjectiveAtRisk
      expr: optimization_request_objective_status{status="at-risk"} == 1
      for: 5m
      labels:
        severity: warning
      annotations:
        summary: "SLO {{ $labels.service_level_objective }} is at risk"

    - alert: SLOObjectiveViolated
      expr: optimization_request_objective_status{status="violated"} == 1
      for: 1m
      labels:
        severity: critical
      annotations:
        summary: "SLO {{ $labels.service_level_objective }} is violated"
```

## Limitations

### Current Version (v0.1.0)

| Limitation | Description | Workaround |
|------------|-------------|------------|
| **Percentage-based SLI only** | SLI query must return a value between 0-100 | Structure your query to return a percentage |
| **Manual PromQL required** | No query templates or builders | Write PromQL directly in the spec |
| **Instant queries only** | Uses Prometheus instant query, not range query | Ensure your query uses `rate()` or similar functions |
| **No multi-cluster support** | One operator per cluster | Deploy SLOK in each cluster |
| **No built-in alerting** | Operator doesn't send alerts directly | Use PrometheusRule for alerting (see example above) |
| **Fixed reconciliation interval** | SLOs are re-evaluated every 1 minute | Cannot be configured per-SLO |

### Query Requirements

Your SLI query **must**:
- Return a single scalar value (not a vector)
- Return a percentage (0-100 scale)
- Use appropriate time functions (`rate()`, `increase()`) for counters

**Good query:**
```promql
(sum(rate(http_requests_total{status=~"2.."}[5m]))
 / sum(rate(http_requests_total[5m]))) * 100
```

**Bad query** (returns vector, not scalar):
```promql
rate(http_requests_total{status=~"2.."}[5m])
```

### Error Budget Calculation

Error budget is calculated as:
```
allowed_errors = (100 - target) * window_in_minutes
consumed = allowed_errors * ((target - actual) / (100 - target))
remaining = allowed_errors - consumed
```

Example for a 99.9% target over 30 days:
- Allowed downtime: 0.1% of 30 days = 43.2 minutes
- If actual is 99.87%, you've consumed ~30% of your budget

## Development

```bash
# Run locally (against current kubeconfig)
make run

# Run tests
make test

# Build Docker image
make docker-build IMG=your-registry/slok:latest

# Deploy to cluster
make deploy IMG=your-registry/slok:latest
```

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.

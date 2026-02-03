# SLOK - Service Level Objectives for Kubernetes
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.20%2B-brightgreen.svg)](https://kubernetes.io)
[![Go Report Card](https://goreportcard.com/badge/github.com/federicolepera/slok)](https://goreportcard.com/report/github.com/federicolepera/slok)

SLOK is a Kubernetes operator that manages Service Level Objectives (SLOs) with automatic error budget tracking. Define your reliability targets as Kubernetes resources, and SLOK will continuously monitor them using Prometheus.

## Quick Start

Get your first SLO running:

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
        query:
          success: sum(increase(http_requests_total{status=~"2.."}[7d]))
          total: sum(increase(http_requests_total[7d]))
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
        query:
          success: sum(increase(http_requests_total{service="payment-api", status=~"2.."}[30d]))
          total: sum(increase(http_requests_total{service="payment-api"}[30d]))
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
        query:
          success: sum(increase(http_request_duration_seconds_bucket{service="checkout", le="0.5"}[7d]))
          total: sum(increase(http_request_duration_seconds_count{service="checkout"}[7d]))
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
        query:
          success: sum(increase(http_requests_total{job="api-gateway", status!~"5.."}[30d]))
          total: sum(increase(http_requests_total{job="api-gateway"}[30d]))

    - name: latency-p99
      target: 99.0
      window: 30d
      sli:
        query:
          success: sum(increase(http_request_duration_seconds_bucket{job="api-gateway", le="0.3"}[30d]))
          total: sum(increase(http_request_duration_seconds_count{job="api-gateway"}[30d]))
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
      status: violated      # met | at-risk | violated | unknown
      errorBudget:
        total: "43.2m"
        consumed: "56.2m"
        remaining: "0.0m"
        percentRemaining: 0.0
      burnRate:
        longBurnRate: 0.5
        shortBurnRate: 0.48
        burnRateThreshold: 14.4
        status: "true"
      lastQueried: "2026-01-28T10:30:00Z"
  lastUpdateTime: "2026-01-28T10:30:00Z"
  conditions:
    - type: Available
      status: "True"
      reason: Reconciled
```

## Alerting

When `alerting.enabled` is set to `true` on an objective, SLOK automatically generates
`PrometheusRule` resources in the same namespace as the SLO. You can configure two
kinds of alerts: error budget alerts and burn rate alerts.

### Budget Alerts

Budget alerts fire when the remaining error budget drops below a given percentage.
If no custom `budgetAlerts` are provided, SLOK creates two default rules:

- **SLOObjectiveAtRisk** (warning) -- remaining budget is between 0% and 10%.
- **SLOObjectiveViolated** (critical) -- remaining budget is at or below 0%.

To override the defaults, specify your own thresholds:

```yaml
objectives:
  - name: availability
    target: 99.9
    window: 30d
    sli:
      query:
        success: sum(increase(http_requests_total{service="payment-api", status=~"2.."}[30d]))
        total: sum(increase(http_requests_total{service="payment-api"}[30d]))
    alerting:
      enabled: true
      budgetAlerts:
        - name: SLOBudgetWarning
          percent: 20        # fires when remaining budget < 20%
          severity: warning
        - name: SLOBudgetCritical
          percent: 5         # fires when remaining budget < 5%
          severity: critical
```

### Burn Rate Alerts

Burn rate alerts use multi-window, multi-burn-rate detection as described in
the [Google SRE Workbook](https://sre.google/workbook/alerting-on-slos/).
The idea is to alert when the error budget is being consumed faster than expected,
rather than waiting for it to run out.

Each burn rate alert defines:

| Field | Description |
|-------|-------------|
| `consumePercent` | Percentage of the total error budget that, if consumed within `consumeWindow`, should trigger an alert. |
| `consumeWindow` | The time frame over which `consumePercent` is evaluated (e.g., `1h`). Together with `consumePercent` and the SLO window, this determines the burn rate threshold. |
| `longWindow` | The long observation window for the `avg_over_time` subquery (e.g., `1h`). |
| `shortWindow` | The short observation window for the `avg_over_time` subquery (e.g., `5m`). Used to confirm the long window signal is not stale. |

Example configuration with two severity tiers:

```yaml
objectives:
  - name: availability
    target: 99.9
    window: 30d
    sli:
      query:
        success: sum(increase(http_requests_total{service="payment-api", status=~"2.."}[30d]))
        total: sum(increase(http_requests_total{service="payment-api"}[30d]))
    alerting:
      enabled: true
      burnRateAlerts:
        - name: HighBurnRate
          consumePercent: 2       # 2% of budget consumed in 1h
          consumeWindow: 1h
          longWindow: 1h
          shortWindow: 5m
          severity: critical
        - name: MediumBurnRate
          consumePercent: 5       # 5% of budget consumed in 6h
          consumeWindow: 6h
          longWindow: 6h
          shortWindow: 30m
          severity: warning
```

The burn rate threshold is calculated as:

```
threshold = (consumePercent / 100) * (sloWindow / consumeWindow)
```

For example, with a 30-day window and `consumePercent: 2`, `consumeWindow: 1h`:

```
threshold = 0.02 * 720h / 1h = 14.4
```

If both the long-window and short-window burn rates exceed 14.4, the alert fires.

### Combining Budget and Burn Rate Alerts

You can use both alert types together on the same objective:

```yaml
alerting:
  enabled: true
  budgetAlerts:
    - name: BudgetLow
      percent: 10
      severity: warning
  burnRateAlerts:
    - name: HighBurnRate
      consumePercent: 2
      consumeWindow: 1h
      longWindow: 1h
      shortWindow: 5m
      severity: critical
```

Budget alerts tell you *how much* budget is left. Burn rate alerts tell you *how fast*
it is being consumed. Using both gives you coverage for slow, sustained degradation
(caught by budget alerts) and sudden spikes (caught by burn rate alerts).

## Limitations

### Current Version

| Limitation | Description | Workaround |
|------------|-------------|------------|
| **Manual PromQL required** | No query templates or builders | Write PromQL directly in the spec |
| **Instant queries only** | Uses Prometheus instant query, not range query | Ensure your query uses `rate()` or similar functions |
| **No multi-cluster support** | One operator per cluster | Deploy SLOK in each cluster |
| **Fixed reconciliation interval** | SLOs are re-evaluated every 1 minute | Cannot be configured per-SLO |
| **Prometheus Operator required for alerts** | PrometheusRule generation requires the Prometheus Operator CRDs | Install the Prometheus Operator or disable `alerting.enabled` |

### Query Requirements

The SLI is defined as a ratio of two PromQL queries: `success` (numerator) and `total` (denominator). The operator computes `(success / total) * 100` to get the actual percentage.

Each query **must**:
- Return a single instant vector value (use `sum()` to aggregate)
- Use `increase(...[window])` where `window` matches the SLO window (e.g., `7d`, `30d`)

Use `increase()` instead of `rate()` so that the error budget reflects accumulated errors
over the entire rolling window. With `rate()`, the error budget would only reflect the
last few minutes and would recover instantly once errors stop. With `increase()`, errors
are tracked over the full window and only "fall off" when they exit the trailing edge
(e.g., after 7 days for a 7-day window).

**Good queries:**
```yaml
# 7-day SLO -- increase range matches the window
sli:
  query:
    success: sum(increase(http_requests_total{status=~"2.."}[7d]))
    total: sum(increase(http_requests_total[7d]))
```

**Bad query** (returns a multi-element vector):
```yaml
sli:
  query:
    success: increase(http_requests_total{status=~"2.."}[7d])
    total: increase(http_requests_total[7d])
```

**Bad query** (instantaneous rate, no rolling window memory):
```yaml
sli:
  query:
    success: sum(rate(http_requests_total{status=~"2.."}[5m]))
    total: sum(rate(http_requests_total[5m]))
```

### Error Budget Calculation

The operator calculates the error budget from the success/total ratio returned by
the SLI queries. Because the queries use `increase(...[window])`, the ratio
represents the actual success rate over the entire rolling window, and the error
budget tracks real accumulated errors.

```
actual            = (success / total) * 100
error_budget      = (100 - target) * window_in_seconds
consumed          = (100 - actual) / 100 * window_in_seconds
remaining         = error_budget - consumed
percent_remaining = (remaining / error_budget) * 100
```

Example for a 99.9% target over 30 days:
- Error budget: 0.1% of 30 days = 43.2 minutes
- If actual is 99.87%, consumed = 0.13% of 30 days = 56.2 minutes
- Remaining = 0 (budget exhausted, status: **violated**)

The budget recovers only when errors exit the trailing edge of the rolling window.
An error that occurred on Monday will stop counting against the budget the following
Monday (for a 7-day window) or 30 days later (for a 30-day window).

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

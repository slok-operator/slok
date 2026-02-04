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
          totalQuery: http_requests_total
          errorQuery: http_requests_total{status=~"5.."}
EOF

# 3. Check the status
kubectl get slo
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

Track the error rate of HTTP requests:

```yaml
apiVersion: observability.slok.io/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: payment-api-availability
spec:
  displayName: "Payment API Availability"
  objectives:
    - name: availability
      target: 99.9        # Target: 99.9% non-error requests
      window: 30d         # Over a 30-day rolling window
      sli:
        query:
          totalQuery: http_requests_total{service="payment-api"}
          errorQuery: http_requests_total{service="payment-api", status=~"5.."}
```

### Latency SLO

Track the percentage of requests above a latency threshold. For histogram-based
latency SLOs, create a recording rule that computes slow requests and reference it
as the error metric:

```yaml
# Prerequisite: create a recording rule for slow requests
# - record: http_request_slow_total
#   expr: |
#     http_request_duration_seconds_count{service="checkout"}
#     - http_request_duration_seconds_bucket{service="checkout", le="0.5"}

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
          totalQuery: http_request_duration_seconds_count{service="checkout"}
          errorQuery: http_request_slow_total{service="checkout"}
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
          totalQuery: http_requests_total{job="api-gateway"}
          errorQuery: http_requests_total{job="api-gateway", status=~"5.."}

    - name: error-rate
      target: 99.0
      window: 30d
      sli:
        query:
          totalQuery: grpc_server_handled_total{job="api-gateway"}
          errorQuery: grpc_server_handled_total{job="api-gateway", grpc_code!="OK"}
```

### Check SLO Status

```bash
# Quick overview with printer columns
kubectl get slo
```

Output:
```
NAME                        DISPLAY NAME               STATUS    ACTUAL   TARGET   BUDGET %   AGE
payment-api-availability    Payment API Availability   met       99.95    99.9     50         30d
api-gateway-slo             API Gateway SLO            warning   99.92    99.95    12.5       15d
```

```bash
# Full status detail
kubectl get slo payment-api-availability -o yaml
```

Output:
```yaml
status:
  objectives:
    - name: availability
      target: 99.9
      actual: 99.87
      status: violated      # met | warning | degraded | critical | violated | unknown
      errorBudget:
        total: "43.2m"
        consumed: "56.2m"
        remaining: "0.0m"
        percentRemaining: 0.0
      burnRate:
        - shortWindow: "5m"
          shortBurnRate: 0.48
          longWindow: "1h"
          longBurnRate: 0.5
        - shortWindow: "1h"
          shortBurnRate: 0.31
          longWindow: "6h"
          longBurnRate: 0.33
        - shortWindow: "6h"
          shortBurnRate: 0.1
          longWindow: "3d"
          longBurnRate: 0.12
        - shortWindow: "7d"
          shortBurnRate: 0.05
          longWindow: "30d"
          longBurnRate: 0.06
      lastQueried: "2026-01-28T10:30:00Z"
  lastUpdateTime: "2026-01-28T10:30:00Z"
  conditions:
    - type: Available
      status: "True"
      reason: Reconciled
```

### Status Values

The objective status is determined by burn rate thresholds (Google SRE Workbook):

| Status | Condition |
|--------|-----------|
| `violated` | Error budget exhausted (remaining <= 0%) |
| `critical` | 5m/1h burn rate both > 14x |
| `degraded` | 1h/6h burn rate both > 6x |
| `warning` | 6h/3d burn rate both > 1x |
| `met` | All burn rates below thresholds |
| `unknown` | Unable to query Prometheus |

## Alerting

SLOK generates `PrometheusRule` resources in the same namespace as the SLO. Each
alert type (`budgetErrorAlerts`, `burnRateAlerts`) is independently enabled via its
own `enabled` flag. PrometheusRules are managed idempotently (`CreateOrUpdate`) and
owned by the SLO resource for automatic garbage collection.

### Budget Alerts

Budget alerts fire when the remaining error budget drops below a given percentage.
When `budgetErrorAlerts.enabled` is `true`, SLOK creates two default rules:

- **SLOObjectiveAtRisk** (warning) -- remaining budget is between 0% and 10%.
- **SLOObjectiveViolated** (warning) -- remaining budget is at or below 0%.

You can also add custom thresholds via `budgetErrorAlerts.alerts`:

```yaml
objectives:
  - name: availability
    target: 99.9
    window: 30d
    sli:
      query:
        totalQuery: http_requests_total{service="payment-api"}
        errorQuery: http_requests_total{service="payment-api", status=~"5.."}
    alerting:
      budgetErrorAlerts:
        enabled: true
        alerts:
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

#### Recording Rules

SLOK generates a set of Prometheus recording rules for each objective:

| Rule | Expression | Purpose |
|------|-----------|---------|
| `slok:sli_error_rate:WINDOW` | `sum(rate(errorQuery[WINDOW])) / sum(rate(totalQuery[WINDOW]))` | Error rate over window |
| `slok:error_budget_target` | `vector(1 - target/100)` | Allowed error fraction |
| `slok:burn_rate:WINDOW` | `slok:sli_error_rate:WINDOW / slok:error_budget_target` | Burn rate factor |

Windows: 5m, 1h, 6h, 3d, 7d, 30d. All error rate rules include zero-traffic
safety (`clamp_min` + `OR` fallback) to avoid NaN when there is no traffic.

#### Default Presets

When `burnRateAlerts.enabled` is `true`, SLOK automatically creates four predefined
alert rules based on the Google SRE Workbook approach:

| Alert | Short Window | Long Window | Burn Rate | Severity | Meaning |
|-------|-------------|-------------|-----------|----------|---------|
| `SLOBurnRateHigh - Critical` | 5m | 1h | >14x | critical | Active outage |
| `SLOBurnRateHigh - Degraded` | 1h | 6h | >6x | warning | High burn |
| `SLOBurnRateHigh - Warning` | 6h | 3d | >1x | warning | Steady erosion |
| `ErrorBudget Finished` | -- | objective window | >1x | warning | Budget exhausted |

Each rule fires when **both** the long-window and short-window burn rates exceed
the threshold. The generated expression uses recording rules:

```
slok:burn_rate:SHORT > threshold AND slok:burn_rate:LONG > threshold
```

#### Custom Burn Rate Alerts

You can also define custom burn rate alerts via `burnRateAlerts.alerts`:

| Field | Description |
|-------|-------------|
| `consumePercent` | Percentage of the total error budget that, if consumed within `consumeWindow`, should trigger an alert. |
| `consumeWindow` | The time frame over which `consumePercent` is evaluated (e.g., `1h`). Together with `consumePercent` and the SLO window, this determines the burn rate threshold. |
| `longWindow` | The long observation window for the burn rate subquery (e.g., `1h`). |
| `shortWindow` | The short observation window for the burn rate subquery (e.g., `5m`). Used to confirm the long window signal is not stale. |

Example configuration with two severity tiers:

```yaml
objectives:
  - name: availability
    target: 99.9
    window: 30d
    sli:
      query:
        totalQuery: http_requests_total{service="payment-api"}
        errorQuery: http_requests_total{service="payment-api", status=~"5.."}
    alerting:
      burnRateAlerts:
        enabled: true
        alerts:
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
  budgetErrorAlerts:
    enabled: true
    alerts:
      - name: BudgetLow
        percent: 10
        severity: warning
  burnRateAlerts:
    enabled: true
    alerts:
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
| **Manual PromQL required** | No query templates or builders | Write metric selectors directly in the spec |
| **Instant queries only** | Uses Prometheus instant query for status, recording rules for SLI | Recording rules handle the rate/window computation |
| **No multi-cluster support** | One operator per cluster | Deploy SLOK in each cluster |
| **Fixed reconciliation interval** | SLOs are re-evaluated every 1 minute | Cannot be configured per-SLO |
| **Prometheus Operator required for alerts** | PrometheusRule generation requires the Prometheus Operator CRDs | Install the Prometheus Operator or disable alerting |
| **Histogram latency SLOs** | Cannot express "slow requests" as a single metric selector | Create a recording rule for slow request count |

### Query Requirements

The SLI is defined by two Prometheus metric selectors: `errorQuery` (error events)
and `totalQuery` (total events). SLOK generates recording rules that compute the
error rate for multiple time windows.

Each field should contain a **metric selector** (metric name with optional label
matchers). The operator wraps these in `sum(rate(...[window]))` when generating
recording rules. Do **not** include `sum()`, `rate()`, or `increase()` in the queries.

**Good queries (metric selectors):**
```yaml
sli:
  query:
    totalQuery: http_requests_total{service="payment-api"}
    errorQuery: http_requests_total{service="payment-api", status=~"5.."}
```

**Bad query** (includes PromQL functions -- the operator adds these automatically):
```yaml
sli:
  query:
    totalQuery: sum(rate(http_requests_total{service="payment-api"}[5m]))
    errorQuery: sum(rate(http_requests_total{service="payment-api", status=~"5.."}[5m]))
```

**Bad query** (returns a multi-element vector -- always use a specific label selector):
```yaml
sli:
  query:
    totalQuery: http_requests_total
    errorQuery: http_requests_total{status=~"5.."}
# This works if there is exactly one matching series; use specific labels to ensure that.
```

The generated recording rules produce these metrics:
```
slok:sli_error_rate:5m   = sum(rate(errorQuery[5m])) / sum(rate(totalQuery[5m]))
slok:sli_error_rate:1h   = sum(rate(errorQuery[1h])) / sum(rate(totalQuery[1h]))
slok:sli_error_rate:6h   = ...
slok:sli_error_rate:3d   = ...
slok:sli_error_rate:7d   = ...
slok:sli_error_rate:30d  = ...
```

The controller queries `slok:sli_error_rate:5m` to compute the current SLI and
error budget, and `slok:burn_rate:*` for burn rate status.

### Error Budget Calculation

The operator calculates the error budget from the error rate returned by the
recording rules:

```
error_rate        = slok:sli_error_rate:5m (from recording rule)
actual            = 100 - (error_rate * 100)
error_budget      = (100 - target) / 100 * window_in_seconds
consumed          = error_rate * window_in_seconds
remaining         = error_budget - consumed
percent_remaining = (remaining / error_budget) * 100
```

Example for a 99.9% target over 30 days:
- Error budget: 0.1% of 30 days = 43.2 minutes
- If error rate is 0.0013 (actual = 99.87%), consumed = 0.13% of 30 days = 56.2 minutes
- Remaining = 0 (budget exhausted, status: **violated**)

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

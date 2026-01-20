# SLOK

SLOK (Service Level Objective for Kubernetes) is a Kubernetes operator for managing Service Level Objectives (SLOs) with automatic error budget tracking.

## Overview

SLOK provides a declarative way to define, monitor, and track SLOs in your Kubernetes cluster. It integrates with Prometheus to query metrics and automatically calculates error budgets, helping teams maintain reliability targets.

## Features

- Custom Resource Definition (CRD) for ServiceLevelObjective
- Prometheus integration for querying SLI metrics
- Automatic error budget calculation
- Status updates with objective tracking
- Query validation (window match checking)

## Installation

### Prerequisites

- Kubernetes cluster v1.28+
- Prometheus deployed in your cluster
- kubectl configured to access your cluster
- Go 1.24+ (for building from source)

### Install CRDs

```bash
make install
```

### Deploy Controller

```bash
make deploy IMG=<your-registry>/slok:latest
```

### Build and Push Image

```bash
make docker-build IMG=<your-registry>/slok:latest
make docker-push IMG=<your-registry>/slok:latest
```

### Uninstall

```bash
make undeploy
make uninstall
```

## Usage

### Define a ServiceLevelObjective

```yaml
apiVersion: observability.slok.io/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: payment-api-availability
spec:
  displayName: "Payment API Availability"
  objectives:
    - name: availability
      target: 99.9
      window: 30d
      sli:
        query: |
          sum(rate(http_requests_total{service="payment-api", code!~"5.."}[30d])) /
          sum(rate(http_requests_total{service="payment-api"}[30d])) * 100
```

### Check SLO Status

```bash
kubectl get slo payment-api-availability -o yaml
```

The status section shows:

```yaml
status:
  objectives:
    - name: availability
      target: 99.9
      actual: 99.87
      status: met
      errorBudget:
        total: "43.2m"
        consumed: "10.5m"
        remaining: "32.7m"
        percentRemaining: 75.69
      lastQueried: "2026-01-20T10:30:00Z"
  lastUpdateTime: "2026-01-20T10:30:00Z"
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PROMETHEUS_URL` | Prometheus server URL | `http://prometheus:9090` |
| `RECONCILE_INTERVAL` | How often to reconcile SLOs | `1m` |

## Architecture

```
+------------------+       +-------------+       +------------+
| ServiceLevel     |       |   SLOK      |       | Prometheus |
| Objective CRD    | ----> | Controller  | ----> |   Server   |
+------------------+       +-------------+       +------------+
                                 |
                                 v
                          +-------------+
                          | Error Budget|
                          | Calculator  |
                          +-------------+
```

## Development

### Build

```bash
make build
```

### Run Tests

```bash
make test
```

### Run Locally

```bash
make install    # Install CRDs
make run        # Run controller locally
```

### Lint

```bash
make lint
```

### Run E2E Tests

```bash
make test-e2e
```

## Roadmap

### v0.1.0 - MVP (Current)

**Objective:** Basic working operator with percentage-based metrics

**Features:**
- CRD ServiceLevelObjective with base spec
- Controller that reconciles SLOs
- Prometheus client integration
- Error budget calculation (percentage-based)
- Status update with objectives
- Query validation (window match check)
- Basic alerting (log when budget < threshold)
- Unit tests for error budget calculator
- Unit tests for prometheus client
- Sample CR

**Limitations:**
- Only percentage-based SLI (0-100)
- Manual query (user writes PromQL)
- Window must match between spec and query
- Alert only in logs (no webhook)
- Instant query (no range query)

---

### v0.2.0 - Multi-type SLI

**Objective:** Threshold-based SLI support (absolute latency, etc.)

**Features:**
- Add `type` field to SLI (percentage | threshold)
- Add `operator` field to SLI (<, >, <=, >=)
- Controller logic for threshold-based SLI
- Error budget calculation for threshold SLI
- Range query support (calculation over entire window)
- Prometheus retention check
- Integration tests with Prometheus mock

**CRD Changes:**
```yaml
sli:
  type: threshold
  operator: "<"
  query: "histogram_quantile(0.95, ...)"
```

---

### v0.3.0 - Alerting and Observability

**Objective:** Real alerts and operator metrics

**Features:**
- Webhook alerting (Slack integration)
- PagerDuty integration
- Generic webhook support
- Burn rate alerting (fast/slow burn detection)
- Operator metrics (Prometheus metrics for the operator itself)
  - `slok_slo_status{name, status}` (met/violated/at-risk)
  - `slok_error_budget_remaining{name}`
  - `slok_reconcile_duration_seconds`
- ServiceMonitor for the operator
- Alert template customization

**CRD Changes:**
```yaml
alerting:
  errorBudgetThreshold: 10
  channels:
    - type: slack
      webhook: "https://hooks.slack.com/..."
    - type: pagerduty
      integrationKey: "xxx"
  burnRate:
    enabled: true
    fastBurnThreshold: 14.4
    slowBurnThreshold: 1
```

---

### v0.4.0 - Template Library

**Objective:** Simplify SLO creation with predefined templates

**Features:**
- Template system for common queries
- Built-in templates:
  - http-availability
  - http-latency-percentile
  - http-latency-threshold
  - grpc-availability
  - grpc-latency
  - job-success-rate
  - queue-processing-latency
- Custom template support (ConfigMap)
- Template validation
- Template documentation generator

**CRD Changes:**
```yaml
sli:
  template: http-availability
  params:
    service: payment-api
    errorCodes: "5.."
    namespace: production
```

---

### v0.5.0 - Recording Rules Support

**Objective:** Integration with Prometheus recording rules

**Features:**
- Auto-detection of recording rules
- Recording rule preference over raw query
- PrometheusRule CRD generation (optional)
- Recording rule templates
- Validation that recording rule exists

**CRD Changes:**
```yaml
sli:
  recordingRule: "slo:availability:30d"
  query: "..."  # fallback if recording rule doesn't exist
```

---

### v0.6.0 - Multi-Window and Reporting

**Objective:** Multiple window support and automatic reports

**Features:**
- Multi-window support (7d, 30d, 90d simultaneously)
- Calendar-based windows (monthly, quarterly)
- Report generation (HTML/PDF)
- Email report scheduling
- Trend analysis (improving/degrading)
- SLOReport CRD for storing reports
- Comparison views (this month vs last)

**New CRD: SLOReport**
```yaml
apiVersion: observability.slok.io/v1alpha1
kind: SLOReport
metadata:
  name: payment-api-jan-2026
spec:
  sloRef: payment-api-slo
  period: "2026-01-01/2026-01-31"
  format: pdf
status:
  reportURL: "s3://reports/payment-api-jan-2026.pdf"
```

---

### v0.7.0 - Policy Enforcement

**Objective:** Deploy gates and policy automation

**Features:**
- Policy enforcement engine
- ArgoCD integration (block sync when budget exhausted)
- Flux integration
- Jenkins/GitLab CI integration
- Manual override mechanism
- Audit log for policy decisions
- Exemption system (emergency deploys)

**CRD Changes:**
```yaml
enforcement:
  enabled: true
  blockDeployOnBudgetExhausted: true
  integrations:
    - type: argocd
      namespace: argocd
    - type: flux
      namespace: flux-system
  exemptions:
    - reason: "Security hotfix"
      approvedBy: "john@example.com"
      expiresAt: "2026-01-25T10:00:00Z"
```

---

### v0.8.0 - Dashboard Auto-Generation

**Objective:** Automatic Grafana dashboards

**Features:**
- Grafana API integration
- Auto-generate dashboard per SLO
- Dashboard templates (overview, detailed, executive)
- Multi-SLO overview dashboard
- Custom panel configuration
- Dashboard versioning

---

### v0.9.0 - Advanced Features

**Objective:** Enterprise features

**Features:**
- Multi-cluster support
- Cost analysis (cost to improve SLO by 0.1%)
- AI-powered recommendations
- Incident correlation (link SLO violations to incidents)
- Capacity planning hints
- Dependency tracking (SLO dependencies)
- Historical playback (what-if scenarios)

---

### v1.0.0 - Production Ready

**Objective:** Stable release

**Features:**
- API stabilization (v1)
- Performance optimization
- Security audit
- Comprehensive documentation
- Migration path from competitors (Sloth, Pyrra)
- Helm chart in artifact hub
- OpenShift OperatorHub submission

## Contributing

Contributions are welcome. Please open an issue first to discuss what you would like to change.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- [Kubebuilder](https://kubebuilder.io/) - SDK for building Kubernetes APIs
- [Sloth](https://github.com/slok/sloth) - Inspiration for SLO management
- [Pyrra](https://github.com/pyrra-dev/pyrra) - Another excellent SLO operator

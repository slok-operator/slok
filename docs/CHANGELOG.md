# Changelog

All notable changes to the SLOK project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [v0.1.0] - 2026-01-30

### Added

#### Custom Resource Definition
- `ServiceLevelObjective` CRD (`observability.slok.io/v1alpha1`) with full OpenAPI validation
- `SLI` type supporting two measurement modes:
  - `percentage`: SLI query returns a value between 0-100
  - `threshold`: SLI query returns a raw value compared against target using an operator (`<`, `>`, `<=`, `>=`)
- CEL validation rule enforcing that `operator` is required when SLI type is `threshold`
- Support for multiple objectives per SLO resource
- Window-based evaluation with configurable time windows (`30d`, `7d`, etc.)

#### Controller
- `ServiceLevelObjectiveReconciler` with 1-minute reconciliation interval
- Prometheus instant query integration for SLI evaluation
- Configurable Prometheus URL via `PROMETHEUS_URL` environment variable (defaults to `http://localhost:9090`)
- Status subresource updates with per-objective results including:
  - Actual SLI value
  - Objective status (`met`, `at-risk`, `violated`, `unknown`)
  - Error budget details (total, consumed, remaining, percent remaining)
  - Last queried timestamp
- Kubernetes conditions support (`Available`, `Progressing`, `Degraded`)
- Leader election support for high-availability deployments

#### Error Budget Calculator
- Percentage-based error budget calculation:
  - Total budget = `(100 - target) * window_in_minutes`
  - Consumed budget based on deviation from target
  - Remaining budget clamped to zero when exhausted
- Threshold-based error budget calculation:
  - Support for `<`, `>`, `<=`, `>=` operators
  - Budget expressed in the same unit as the target value
- Status determination logic:
  - `met`: SLI meets target and budget > 10%
  - `at-risk`: SLI meets target but budget <= 10%
  - `violated`: SLI does not meet target or budget exhausted

#### Prometheus Metrics
- Custom metrics exposed for Grafana dashboards:
  - `slo_objective_target` (gauge): target value per objective
  - `slo_objective_actual` (gauge): actual SLI value per objective
  - `slo_objective_error_budget_remaining_percent` (gauge): remaining error budget percentage
  - `slo_objective_status` (gauge): objective status as numeric value
- Metrics labeled with `service_level_objective` and `objective_name`

#### Webhooks
- Validating admission webhook for `ServiceLevelObjective` resources
- Webhook triggered on create and update operations

#### Helm Chart
- Full Helm chart (`charts/slok/`) for production deployment
- Configurable values:
  - Prometheus URL
  - Webhook enable/disable
  - cert-manager integration (optional)
  - PrometheusRule deployment (optional)
  - ServiceMonitor deployment (optional)
  - Resource limits and requests
  - Replica count
- Includes CRDs, RBAC (ClusterRole, Role for leader election), ServiceAccount
- Conditional webhook certificates via cert-manager
- PrometheusRule template with `SLOObjectiveAtRisk` (warning) and `SLOObjectiveViolated` (critical) alerts

#### Grafana
- Grafana dashboard JSON for SLO visualization

#### CI/CD
- GitHub Actions workflow for Docker image build and push
- Multi-platform support (linux/amd64, linux/arm64) using native Go cross-compilation (`FROM --platform=$BUILDPLATFORM`)
- Lint and test workflows

#### Testing
- Unit tests for error budget calculator (percentage and threshold modes)
- Unit tests for status determination logic
- Controller integration tests

#### Kubernetes Manifests
- Example application deployment (`k8s/deployment.yaml`) with ServiceMonitor
- Additional scrape config for local development with Minikube
- PrometheusRule for SLO alerting (`k8s/prometheus-rule.yaml`)

### Known Limitations
- SLI percentage queries must return a single scalar value between 0-100
- Instant queries only (no range query support)
- Fixed 1-minute reconciliation interval (not configurable per-SLO)
- Single-cluster support only
- No built-in alerting (relies on PrometheusRule)
- Manual PromQL required (no query templates or builders)

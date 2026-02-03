# Changelog

All notable changes to the SLOK project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [v0.2.0] - 2026-02-03

### Added

#### `$window` Placeholder in SLI Queries
- The controller now resolves `$window` in SLI `success` and `total` queries, replacing it with the objective's `window` value (e.g., `7d`, `30d`)
- Burn rate queries also resolve `$window` per preset window (short/long)
- Users can write queries like `sum(increase(http_requests_total[$window]))` without hardcoding the window

#### Multi-Window Burn Rate Calculation
- Added 4 default burn rate presets based on the Google SRE Workbook:
  - 5m/1h (>14x, outage)
  - 1h/6h (>6x, high burn)
  - 6h/3d (>1x, erosion)
  - 7d/30d (>0.5x, slow burn)
- Burn rate values rounded to 2 decimal places
- Each objective now reports burn rate metrics for all presets in its status

#### PrometheusRule Generation
- Automatic generation of PrometheusRule resources for burn rate alerts when `burnRateAlerts.enabled: true`
- Each preset generates a dual-condition expression: `(1 - (success/total)) / (1 - (target/100)) > burnRate` for both long and short windows
- Alert names: `SLOBurnRateOutage` (critical), `SLOBurnRateHigh` (critical), `SLOBurnRateErosion` (warning), `SLOBurnRateSlow` (warning)
- Budget error alerts with default `SLOObjectiveAtRisk` and `SLOObjectiveViolated` rules when `budgetErrorAlerts.enabled: true`
- Support for custom budget threshold alerts via `budgetErrorAlerts.alerts`

#### Prometheus Metrics
- Added `slo_objective_status` gauge metric with status label

### Changed

#### CRD Structure
- `Alerting` now uses separate `budgetErrorAlerts` and `burnRateAlerts` sections, each with its own `enabled` flag
- `BurnRateStatus` in `ObjectiveStatus` changed from single struct to `[]BurnRateStatus` array (one entry per burn rate preset)
- `BurnRateStatus` fields updated: removed `burnRateThreshold` and `status`, added `longWindow` and `shortWindow`

#### SLI Queries
- Switched recommended query pattern from `rate(...[5m])` to `increase(...[$window])` for proper rolling-window error budget tracking
- Error budget now reflects accumulated errors over the full window and recovers only when errors exit the trailing edge

#### Error Budget
- `DetermineStatus` function moved to `errorbudget` package (also available in `burnrate` package)

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

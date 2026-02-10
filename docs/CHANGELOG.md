# Changelog

All notable changes to the SLOK project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

#### Event Correlation
- New `SLOCorrelation` CRD that records burn rate spikes and correlated cluster changes
  - Short name: `slocorr` for `kubectl get slocorr`
  - Printer columns: SLO, Severity, Burn Rate, Events, Detected, Age
  - Status includes: correlated events, severity, time window, human-readable summary
- Automatic correlation analysis triggered when burn rate spikes are detected
- Change collector watches Deployments, ConfigMaps, Secrets, and Events in the cluster
- Anomaly detector identifies burn rate spikes using rolling window comparison
- Correlation engine scores changes based on:
  - Time proximity to the spike (±30 minutes window)
  - Namespace matching
  - Resource type (Deployments scored higher than Events)
  - Label selector matching
- Confidence levels (high/medium/low) assigned to each correlated event
- `workloadSelector` field added to `ServiceLevelObjective` spec for filtering correlation:
  - `labelSelector`: only include resources with matching labels
  - `namespaces`: limit correlation to specific namespaces (defaults to SLO namespace)
- Watched resources track:
  - Deployments: container images
  - ConfigMaps: key count changes
  - Secrets: type and key count (values never exposed)
  - Events: CrashLoopBackOff, OOMKilled, FailedScheduling, ImagePullBackOff, etc.
- Optional LLM-enhanced summary via Groq API (`GROQ_API_KEY` environment variable):
  - Uses Llama 3.3 70B to refine the correlation summary with smarter root cause analysis
  - 30-second timeout with automatic fallback to rule-based summary on failure
  - Prioritizes capacity-reducing events over secondary symptoms (probe failures, pod churn)

#### SLI Templates
- Built-in templates for common SLI patterns that generate PromQL automatically:
  - `http-availability`: HTTP request success rate (non-5xx responses) using `http_requests_total`
  - `http-latency`: HTTP request latency using histogram buckets with configurable `threshold` parameter
  - `kubernetes-apiserver`: Kubernetes API server availability using `apiserver_request_total` with configurable `errorCodes` parameter
- Template resolution engine in `internal/templates/` package
- `RawExpr` support for complex PromQL expressions with `{{window}}` placeholder (used by `http-latency`)
- Zero-traffic safety in all template-generated queries using `clamp_min(..., 1e-12)`
- `Params` field added to `TemplateStruct` for template-specific configuration
- Comprehensive test coverage for all templates (21 tests)

### Changed

- Refactored `prometheus_rule_generator.go` to use helper functions and loops, reducing code duplication
- Updated README with SLI Templates documentation and examples
- Updated README with Event Correlation documentation including SLOCorrelation CRD, workloadSelector, and confidence scoring
- Removed "Manual PromQL required" from limitations (templates are now available)
- Controller now integrates with correlation engine to create SLOCorrelation resources on burn rate spikes
- Added RBAC permissions for watching Deployments, ConfigMaps, Secrets, and Events

### Fixed

- Fixed ineffassign lint issue in controller reconciliation loop
- Fixed comment spacing issues in burnrate calculator
- Fixed gofmt formatting in types.go

## [v0.2.0] - 2026-02-04

### Added

#### Recording Rules
- SLOK now generates Prometheus recording rules for each objective:
  - `slok:sli_error_rate:WINDOW` for 6 windows (5m, 1h, 6h, 3d, 7d, 30d)
  - `slok:objective_target` and `slok:error_budget_target` constants
  - `slok:burn_rate:WINDOW` for each window
- Zero-traffic safety: error rate rules use `clamp_min(..., 1e-12)` with `OR` fallback to avoid NaN/division-by-zero when there is no traffic
- Burn rate rules use `on (slo_name, slo_namespace, objective_name)` for label matching

#### Multi-Window Burn Rate Calculation
- Added 4 default burn rate presets based on the Google SRE Workbook:
  - 5m/1h (>14x, outage)
  - 1h/6h (>6x, high burn)
  - 6h/3d (>1x, erosion)
  - 7d/30d (>0.5x, slow burn)
- Each objective now reports burn rate metrics for all presets in its status

#### PrometheusRule Generation
- Automatic generation of PrometheusRule resources with recording rules and burn rate alerts when `burnRateAlerts.enabled: true`
- Alert expressions use recording rules: `slok:burn_rate:SHORT > threshold AND slok:burn_rate:LONG > threshold`
- Budget error alerts with default `SLOObjectiveAtRisk` and `SLOObjectiveViolated` rules when `budgetErrorAlerts.enabled: true`
- Support for custom budget threshold alerts via `budgetErrorAlerts.alerts`
- PrometheusRules managed idempotently with `CreateOrUpdate` and `SetControllerReference` for automatic garbage collection

#### CRD Enhancements
- Added `shortName: slo` for `kubectl get slo` support
- Added printer columns: Display Name, Status, Actual, Target, Budget %, Age

#### Prometheus Metrics
- Added `slo_objective_status` gauge metric with status label

### Changed

#### SLI Query Model (Breaking)
- **Breaking**: Renamed query fields from `success`/`total` to `errorQuery`/`totalQuery`
- The SLI is now computed as an error rate: `error_rate = errorQuery / totalQuery`
- Queries should be Prometheus metric selectors (e.g., `http_requests_total{status=~"5.."}`), not full PromQL expressions; the operator wraps them in `sum(rate(...[window]))` via recording rules
- Removed `$window` placeholder -- recording rules handle all time windows internally

#### Status Determination (Breaking)
- **Breaking**: Removed `at-risk` status
- Added new statuses based on multi-window burn rates: `warning`, `degraded`, `critical`
- Status is now determined by burn rate thresholds:
  - `violated`: error budget exhausted (budget <= 0%)
  - `critical`: 5m/1h burn rate both > 14x
  - `degraded`: 1h/6h burn rate both > 6x
  - `warning`: 6h/3d burn rate both > 1x
  - `met`: all burn rates below thresholds

#### CRD Structure
- `Alerting` now uses separate `budgetErrorAlerts` and `burnRateAlerts` sections, each with its own `enabled` flag
- `BurnRateStatus` in `ObjectiveStatus` changed from single struct to `[]BurnRateStatus` array (one entry per burn rate preset)
- `BurnRateStatus` field order: `shortWindow`, `shortBurnRate`, `longWindow`, `longBurnRate`

#### Error Budget
- `DetermineStatus` function moved to `errorbudget` package
- `Calculate` now takes a single `sliErrorRate` parameter (error rate ratio 0-1) instead of separate success/total values

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

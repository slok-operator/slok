# PrometheusRule issues to track

This document lists the PrometheusRule-related issues found during the review of Slok's generated rules.

## 1. Burn-rate alert expressions do not match short and long window series correctly

### Problem

Burn-rate alert expressions compare two recording rules with different `slok_window` labels, for example:

```promql
slok:burn_rate:5m{...} > 14 and slok:burn_rate:1h{...} > 14
```

The two series differ by:

```text
slok_window="5m"
slok_window="1h"
```

PromQL binary/logical operators match series by labels by default. Because `slok_window` is different, the `and` expression can return an empty result, meaning burn-rate alerts may never fire even when both conditions are true.

### Suggested fix

Use explicit vector matching, ignoring `slok_window` and matching only the SLO identity labels:

```promql
slok:burn_rate:5m{...} > 14
and on (slo_name, slo_namespace, objective_name)
slok:burn_rate:1h{...} > 14
```

### Acceptance criteria

- Burn-rate alert expressions use explicit `and on (...)` vector matching.
- Alerts fire when both short and long burn-rate windows exceed the threshold.
- Tests cover the generated PromQL expression.

---

## 2. Generated PromQL uses uppercase `AND` / `OR`

### Problem

Some generated PromQL expressions use uppercase operators:

```promql
... OR ...
... AND ...
```

PromQL examples and canonical syntax use lowercase operators:

```promql
... or ...
... and ...
```

Even if some Prometheus versions accept uppercase operators, generating canonical lowercase PromQL is safer and less surprising.

### Suggested fix

Generate lowercase PromQL operators consistently:

```promql
or
and
unless
```

### Acceptance criteria

- Generated rules use lowercase PromQL logical operators.
- Existing PrometheusRule tests are updated accordingly.
- PrometheusRule validation still passes.

---

## 3. Documented slow-burn preset is missing from generated defaults

### Problem

The burn-rate preset documentation/comment mentions a slow-burn alert such as:

```text
7d / 30d > 0.5x
```

but the default burn-rate presets currently include only:

```text
5m / 1h  > 14
1h / 6h  > 6
6h / 3d  > 1
```

This creates a mismatch between documented behavior and generated rules.

### Suggested fix

Either:

- add the missing `7d / 30d > 0.5x` preset, or
- remove/update the documentation/comment if the preset is intentionally unsupported.

### Acceptance criteria

- Documentation/comments and generated default presets are consistent.
- Tests verify the expected number of default burn-rate alerts.

---

## 4. Burn-rate alert `for` durations are defined but not applied

### Problem

The burn-rate preset struct contains a `For` field, but generated Prometheus alert rules do not set the Prometheus `for` duration.

As a result, burn-rate alerts can fire immediately once the expression is true, instead of requiring the condition to remain true for the intended duration.

### Suggested fix

When generating alert rules, assign the preset `For` value to the PrometheusRule `Rule.For` field.

### Acceptance criteria

- Burn-rate alert rules include the expected `for` duration.
- Tests verify `for` is rendered for default burn-rate alerts.

---

## 5. Custom burn-rate alert configuration is currently ignored

### Problem

The CRD exposes custom burn-rate alert configuration:

```yaml
alerting:
  burnRateAlerts:
    enabled: true
    alerts:
      - name: ...
        consumePercent: ...
        consumeWindow: ...
        longWindow: ...
        shortWindow: ...
        severity: ...
```

However, the generator currently uses the default burn-rate presets and does not appear to apply the custom alert values from the CR spec.

### Suggested fix

Support user-defined burn-rate alerts when `spec.objective.alerting.burnRateAlerts.alerts` is provided.

Possible behavior:

- use custom alerts if provided;
- fall back to defaults if enabled but no custom alerts are provided.

### Acceptance criteria

- Custom burn-rate alert definitions are reflected in generated PrometheusRule alerts.
- Default presets are still generated when no custom alerts are provided.
- Tests cover both default and custom burn-rate alert generation.

---

## 6. Budget-percent alert expression references a wrong/nonexistent metric name

### Problem

Budget-percent alerts appear to reference:

```promql
optimization_request_objective_percent_remaining
```

but the metric exposed by Slok appears to be:

```promql
slok_slo_objective_percent_remaining
```

If this is correct, generated budget-percent alerts will never fire because they point to a nonexistent metric.

### Suggested fix

Update the alert expression to use the actual metric name, or preferably generate budget alerts from existing recording rules where possible.

### Acceptance criteria

- Budget-percent alerts reference an existing metric.
- Tests verify the generated expression uses the expected metric name.
- A sample budget alert can be evaluated successfully in Prometheus.

---

## 7. Target validation allows values above 100%

### Problem

The CRD currently allows objective targets up to `200`:

```go
// +kubebuilder:validation:Maximum=200
Target float64 `json:"target"`
```

For availability/error-budget math, targets above `100` produce a negative error budget:

```text
error_budget = 1 - target/100
```

For example:

```text
target = 150
error_budget = -0.5
```

This breaks burn-rate calculations and alert semantics.

### Suggested fix

Restrict objective targets to `0 <= target <= 100`, unless there is a specific objective type where values above `100` are valid.

### Acceptance criteria

- CRD validation rejects targets above `100` for percentage-based objectives.
- Tests cover invalid target values.
- Existing samples remain valid.

---

## 8. Missing metric behavior should be explicit and tested

### Problem

Generated SLI error-rate expressions use fallback logic for missing metrics, including `absent(...)`.

This behavior is important because missing `total` series can be interpreted as an error condition. That may be intentional, but it should be explicit and covered by tests.

### Suggested fix

Document and test the intended semantics for:

- no error series but total series exists;
- no total series;
- total series exists but traffic is zero.

### Acceptance criteria

- README/docs explain missing-metric behavior.
- Unit tests verify generated PromQL fallback behavior.
- Sample documentation warns about label mismatches and scrape gaps.

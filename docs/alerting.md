# Alerting and recording rules

SloK generates Prometheus recording rules and alerts for each SLO.

## Recording rules

For each objective, SloK generates SLI error-rate rules over several windows:

```text
slok:sli_error_rate:5m
slok:sli_error_rate:1h
slok:sli_error_rate:6h
slok:sli_error_rate:3d
slok:sli_error_rate:7d
slok:sli_error_rate:30d
```

It also generates error-budget and burn-rate rules:

```text
slok:error_budget_target
slok:burn_rate:5m
slok:burn_rate:1h
slok:burn_rate:6h
slok:burn_rate:3d
slok:burn_rate:7d
slok:burn_rate:30d
```

Manual query selectors are wrapped with Prometheus functions, for example:

```promql
sum(rate(errorQuery[WINDOW])) / clamp_min(sum(rate(totalQuery[WINDOW])), 1e-12)
```

## Error budget

SloK computes error budget from the target and objective window.

For a `99.9` target over `30d`, the allowed error budget is `0.1%` of 30 days,
which is 43.2 minutes.

Conceptually:

```text
error_rate         = measured error fraction
actual             = 100 - (error_rate * 100)
error_budget       = (100 - target) / 100 * window_seconds
consumed           = error_rate * window_seconds
remaining          = error_budget - consumed
percent_remaining  = remaining / error_budget * 100
```

## Burn-rate alerts

Burn-rate alerts follow the multi-window, multi-burn-rate approach from the
[Google SRE Workbook](https://sre.google/workbook/alerting-on-slos/).

The idea is to alert when the error budget is being consumed too quickly, instead
of waiting until it is already gone.

Default alert types cover:

- fast burn / active outage
- medium burn / degraded behavior
- slow burn / steady erosion
- exhausted error budget

Each burn-rate alert checks both a short and a long window to reduce noise.

## Custom alerting

Alerting is configured under `spec.objective.alerting`.

Budget alerts:

```yaml
alerting:
  budgetErrorAlerts:
    enabled: true
    alerts:
      - name: SLOBudgetWarning
        percent: 20
        severity: warning
      - name: SLOBudgetCritical
        percent: 5
        severity: critical
```

Burn-rate alerts:

```yaml
alerting:
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

The burn-rate threshold is calculated as:

```text
threshold = (consumePercent / 100) * (sloWindow / consumeWindow)
```

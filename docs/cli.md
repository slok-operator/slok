# CLI backtesting

The `slok` CLI can backtest SLOs against historical Prometheus data.

Build it locally:

```bash
make build-cli
```

Run it from the repository:

```bash
bin/slok backtest --help
```

## Existing SLO mode

Use this mode when the SLO has already been applied and SloK recording rules exist
in Prometheus.

```bash
bin/slok backtest \
  --namespace default \
  --name checkout-availability \
  --prometheus-url http://localhost:9090
```

This queries the generated recording rule:

```promql
avg_over_time(slok:sli_error_rate:5m{...}[range])
```

## Pre-apply YAML mode

Use `--pre-apply` to test an SLO YAML before applying it to the cluster.

```bash
bin/slok backtest \
  -f slo.yaml \
  --pre-apply \
  --prometheus-url http://localhost:9090
```

The YAML must contain manual SLI queries:

```yaml
spec:
  objective:
    sli:
      query:
        totalQuery: http_requests_total{service="checkout"}
        errorQuery: http_requests_total{service="checkout",status=~"5.."}
```

The CLI builds a Prometheus query directly from those selectors:

```promql
sum(increase(errorQuery[range])) / clamp_min(sum(increase(totalQuery[range])), 1e-12)
```

This mode does not require the SLO or its recording rules to exist yet.

Current limitation: pre-apply mode supports manual `totalQuery` / `errorQuery` SLIs.
Template-based pre-apply backtesting is planned.

## What-if targets

Compare several candidate targets in one run:

```bash
bin/slok backtest \
  -f slo.yaml \
  --pre-apply \
  --targets 99,99.5,99.9,99.95 \
  --prometheus-url http://localhost:9090
```

## Range selection

If `--range` is omitted, the CLI uses `spec.objective.window` from the SLO.

```bash
bin/slok backtest -f slo.yaml --pre-apply --range 7d
```

## Timeout

The CLI uses a timeout for Kubernetes and Prometheus requests.

```bash
bin/slok backtest -f slo.yaml --pre-apply --timeout 45s
```

Default timeout: `30s`.

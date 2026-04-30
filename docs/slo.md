# SLO definitions

`ServiceLevelObjective` is the core SloK resource. It defines a single reliability
objective for a service.

```yaml
apiVersion: observability.slok.io/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: checkout-availability
  namespace: default
spec:
  displayName: Checkout Availability
  objective:
    name: availability
    target: 99.9
    window: 30d
    sli:
      query:
        totalQuery: http_requests_total{service="checkout"}
        errorQuery: http_requests_total{service="checkout",status=~"5.."}
```

Apply it:

```bash
kubectl apply -f slo.yaml
```

Check status:

```bash
kubectl get slo
kubectl get slo checkout-availability -o yaml
```

## Objective fields

| Field | Description |
|-------|-------------|
| `name` | Objective name inside the SLO |
| `target` | Desired success percentage, for example `99.9` |
| `window` | Rolling objective window, for example `7d` or `30d` |
| `sli` | How SloK measures the objective |

## Manual SLI queries

Manual SLIs use two Prometheus metric selectors:

- `totalQuery`: total events
- `errorQuery`: failed events

```yaml
sli:
  query:
    totalQuery: http_requests_total{service="payment-api"}
    errorQuery: http_requests_total{service="payment-api",status=~"5.."}
```

Queries should be metric selectors only. Do not include `sum()`, `rate()`, or
`increase()`. SloK wraps these selectors when generating recording rules.

Good:

```yaml
totalQuery: http_requests_total{service="payment-api"}
errorQuery: http_requests_total{service="payment-api",status=~"5.."}
```

Avoid:

```yaml
totalQuery: sum(rate(http_requests_total{service="payment-api"}[5m]))
errorQuery: sum(rate(http_requests_total{service="payment-api",status=~"5.."}[5m]))
```

## Templates

Templates generate the SLI queries for common patterns.

```yaml
sli:
  template:
    name: http-availability
    labels:
      service: payment-api
```

Available templates:

| Template | Description | Required params |
|----------|-------------|-----------------|
| `http-availability` | HTTP request success rate, based on non-5xx responses | none |
| `http-latency` | HTTP latency SLO based on histogram buckets | `threshold` |
| `kubernetes-apiserver` | Kubernetes API server availability | none |

### HTTP availability

```yaml
sli:
  template:
    name: http-availability
    labels:
      service: payment-api
```

Generated selectors:

```yaml
totalQuery: http_requests_total{service="payment-api"}
errorQuery: http_requests_total{service="payment-api",status=~"5.."}
```

### HTTP latency

```yaml
sli:
  template:
    name: http-latency
    labels:
      service: checkout
    params:
      threshold: "0.5"
```

The `threshold` value is expressed in seconds.

### Kubernetes API server

```yaml
sli:
  template:
    name: kubernetes-apiserver
    labels:
      verb: GET
      resource: pods
    params:
      errorCodes: "5.."
```

## Status

SloK writes the current objective state to the SLO status, including:

- actual availability
- objective status
- total, consumed, and remaining error budget
- burn-rate windows
- last query timestamp

Status values:

| Status | Meaning |
|--------|---------|
| `met` | Objective is healthy |
| `warning` | Slow burn-rate warning |
| `degraded` | Medium burn-rate degradation |
| `critical` | Fast burn-rate / likely active outage |
| `violated` | Error budget exhausted |
| `unknown` | SloK could not query or compute the objective |

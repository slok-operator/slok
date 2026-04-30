# SLO composition

`SLOComposition` lets you define a higher-level reliability target from multiple
existing SLOs.

This is useful for user journeys where success depends on several services, for
example a checkout flow that depends on frontend, cart, payments, and inventory.

## Supported strategies

| Strategy | Description | Status |
|----------|-------------|--------|
| `AND_MIN` | The composition is only as healthy as the weakest referenced SLO | Stable |
| `WEIGHTED_ROUTES` | Models traffic flowing through weighted service chains | Alpha |

## AND_MIN

`AND_MIN` uses the worst referenced SLO as the composed signal.

```yaml
apiVersion: observability.slok.io/v1alpha1
kind: SLOComposition
metadata:
  name: checkout-flow
  namespace: default
spec:
  target: 99.9
  window: 30d
  objectives:
    - name: frontend
      ref:
        name: frontend-availability
    - name: payments
      ref:
        name: payments-availability
  composition:
    type: AND_MIN
```

## WEIGHTED_ROUTES

`WEIGHTED_ROUTES` models systems where requests can take different paths.

For example, 90% of users may follow the standard checkout path, while 10% use a
coupon service. Each route has a weight, and each route has a chain of services.

```yaml
apiVersion: observability.slok.io/v1alpha1
kind: SLOComposition
metadata:
  name: checkout-flow
spec:
  target: 99.9
  window: 30d
  objectives:
    - name: base
      ref:
        name: checkout-base-availability
    - name: coupon
      ref:
        name: coupon-availability
    - name: payments
      ref:
        name: payments-availability
  composition:
    type: WEIGHTED_ROUTES
    params:
      routes:
        - name: no-coupon
          weight: 0.9
          chain:
            - base
            - payments
        - name: with-coupon
          weight: 0.1
          chain:
            - base
            - coupon
            - payments
```

The composed error rate is calculated from the weighted success probability of each
route.

`WEIGHTED_ROUTES` is currently alpha and may change.

## Status

Check compositions with:

```bash
kubectl get sloc
kubectl get sloc checkout-flow -o yaml
```

SloK writes composed availability, error budget, burn rate, and status to the
composition status.

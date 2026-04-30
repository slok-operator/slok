# Event correlation

SloK can correlate SLO degradation with recent Kubernetes changes.

When a burn-rate spike is detected, SloK creates an `SLOCorrelation` resource that
lists possible causes and confidence levels.

## How it works

1. SloK watches cluster resources and events.
2. It detects burn-rate spikes from SLO status.
3. It looks for relevant changes around the spike window.
4. It scores candidate causes by time proximity, namespace, resource type, and labels.
5. It writes the result to an `SLOCorrelation` resource.

## Check correlations

```bash
kubectl get slocorr
kubectl get slocorr <name> -o yaml
```

## Watched resources

| Resource | What is tracked |
|----------|-----------------|
| Deployment | Container images and replica changes |
| ConfigMap | Key count changes |
| Secret | Secret type and key count, not values |
| Event | CrashLoopBackOff, OOMKilled, FailedScheduling, and similar events |

## Workload selector

Use `workloadSelector` to reduce noise by focusing correlation on resources related
to the SLO.

```yaml
apiVersion: observability.slok.io/v1alpha1
kind: ServiceLevelObjective
metadata:
  name: payment-api-availability
spec:
  displayName: Payment API Availability
  workloadSelector:
    labelSelector:
      app: payment-api
      team: platform
    namespaces:
      - production
      - staging
  objective:
    name: availability
    target: 99.9
    window: 30d
    sli:
      template:
        name: http-availability
        labels:
          service: payment-api
```

Without `workloadSelector`, SloK considers changes in the SLO namespace.

## Optional LLM-enhanced summaries

If `GROQ_API_KEY` is configured, SloK can ask Groq to refine the generated summary.

Without this environment variable, SloK uses the built-in rule-based summary.

```bash
export GROQ_API_KEY="gsk_..."
```

The LLM integration is optional and not required for correlation to work.

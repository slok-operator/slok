# Dashboard

SloK ships with an optional web dashboard.

The dashboard is made of two images:

- `ghcr.io/slok-operator/slok-dashboard-backend`
- `ghcr.io/slok-operator/slok-dashboard-frontend`

It can be installed through the SloK Helm chart.

## Features

- List SLOs across namespaces.
- Filter by namespace, status, labels, and search text.
- Inspect objective status, target, error budget, and burn rate.
- View historical availability and error-budget trends from Prometheus.
- Use fixed or custom time ranges.
- Drag-to-zoom on trend charts.

## Install

```bash
helm install slok charts/slok \
  --namespace slok-system \
  --create-namespace \
  --set dashboard.enabled=true \
  --set dashboard.ingress.enabled=true \
  --set dashboard.ingress.className=nginx \
  --set dashboard.ingress.hosts[0].host=slok-dashboard.local \
  --set dashboard.prometheus.url=http://prometheus-kube-prometheus-prometheus.monitoring.svc:9090
```

If you do not want an Ingress:

```bash
helm install slok charts/slok \
  --namespace slok-system \
  --create-namespace \
  --set dashboard.enabled=true \
  --set dashboard.ingress.enabled=false
```

Then use port-forwarding:

```bash
kubectl port-forward -n slok-system svc/slok-dashboard-frontend 8080:8080
```

## Security model

The dashboard intentionally does not implement authentication.

Protect it using your platform controls, for example:

- authenticated Ingress
- oauth2-proxy
- VPN
- private network access
- internal-only load balancer

## Prometheus

Trend charts require a Prometheus URL:

```yaml
dashboard:
  prometheus:
    url: http://prometheus-kube-prometheus-prometheus.monitoring.svc:9090
```

Without Prometheus, the dashboard can still read SLO objects from Kubernetes, but
historical charts will not work.

![SloK Dashboard](images/dashboard.png)

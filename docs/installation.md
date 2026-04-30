# Installation

SloK can be installed with Helm or Kustomize.

## Requirements

| Requirement | Notes |
|-------------|-------|
| Kubernetes 1.20+ | Required for CRDs and controller-runtime |
| Prometheus 2.x+ | Required for SLI, burn-rate, status, and backtest queries |
| Prometheus Operator | Required for generated `PrometheusRule` resources |
| cert-manager | Required when admission webhooks are enabled |

## Helm

Install the chart from this repository:

```bash
helm install slok charts/slok \
  --namespace slok-system \
  --create-namespace \
  --set prometheus.url=http://prometheus-kube-prometheus-prometheus.monitoring.svc:9090
```

If you are using `kube-prometheus-stack`, Prometheus is commonly available at:

```text
http://prometheus-kube-prometheus-prometheus.monitoring.svc:9090
```

Verify the deployment:

```bash
kubectl get pods -n slok-system
kubectl get crd servicelevelobjectives.observability.slok.io
kubectl get crd slocompositions.observability.slok.io
kubectl get crd slocorrelations.observability.slok.io
```

## Useful Helm values

| Value | Description | Default |
|-------|-------------|---------|
| `image.repository` | Operator image | `ghcr.io/slok-operator/slok` |
| `image.tag` | Operator image tag | `latest` |
| `prometheus.url` | Prometheus URL used by the operator | empty |
| `webhook.enabled` | Enable admission webhooks | `true` |
| `certManager.enabled` | Use cert-manager for webhook certificates | `true` |
| `dashboard.enabled` | Deploy the optional dashboard | `true` |
| `dashboard.ingress.enabled` | Create dashboard Ingress | `true` |
| `dashboard.ingress.className` | Dashboard Ingress class | `nginx` |
| `dashboard.ingress.hosts[0].host` | Dashboard hostname | `slok-dashboard.local` |
| `dashboard.prometheus.url` | Prometheus URL used by dashboard charts | kube-prometheus-stack default |

## Dashboard install example

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

The dashboard does not implement authentication itself. Put it behind your existing
Ingress authentication, oauth2-proxy, VPN, or internal network controls.

## Kustomize

For local development:

```bash
kubectl apply -k config/default
```

To deploy a custom image:

```bash
make deploy IMG=ghcr.io/slok-operator/slok:dev
```

## Disable webhooks for development

```bash
helm install slok charts/slok \
  --namespace slok-system \
  --create-namespace \
  --set webhook.enabled=false \
  --set certManager.enabled=false
```

#!/usr/bin/env bash
set -euo pipefail

# --- Config ---
NAMESPACE="slok"
RELEASE_NAME="slok"
CHART_PATH="$(dirname "$0")/../charts/slok"
GROQ_SECRET_NAME="groq-secret"
PROMETHEUS_URL="http://prometheus-kube-prometheus-prometheus.monitoring.svc:9090"

# --- Minikube ---
echo "Starting minikube cluster..."

# Always delete the existing cluster to ensure a clean state
minikube delete 2>/dev/null || true

minikube start \
  --addons=metrics-server,default-storageclass,storage-provisioner

echo "Minikube cluster is ready."
minikube status

# --- Helm repos ---
echo "Adding Helm repositories..."
helm repo add jetstack https://charts.jetstack.io --force-update
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts --force-update
helm repo update

# --- cert-manager ---
echo "Installing cert-manager..."
helm upgrade --install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true \
  --wait

# --- kube-prometheus-stack ---
echo "Installing kube-prometheus-stack..."
helm upgrade --install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace \
  --set-json 'prometheus.prometheusSpec.ruleNamespaceSelector={}' \
  --set grafana.adminPassword="prom-operator" \
  --wait --timeout=5m

# --- Wait for cert-manager ---
echo "Waiting for cert-manager deployments to roll out..."
kubectl rollout status deployment/cert-manager -n cert-manager --timeout=120s
kubectl rollout status deployment/cert-manager-cainjector -n cert-manager --timeout=120s
kubectl rollout status deployment/cert-manager-webhook -n cert-manager --timeout=120s

echo "Probing cert-manager webhook until it accepts requests..."
until kubectl create --dry-run=server -f - &>/dev/null 2>&1 <<'EOF'
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: probe-issuer
  namespace: default
spec:
  selfSigned: {}
EOF
do
  echo "  Not ready yet, retrying in 3s..."
  sleep 3
done
echo "cert-manager webhook is ready."

# --- CRDs ---
echo "Installing CRDs..."
kubectl apply -f "$(dirname "$0")/../config/crd/bases/"

# --- Namespace ---
echo "Creating namespace ${NAMESPACE} (if not exists)..."
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

# --- Groq Secret ---
echo "Creating Groq API key secret..."
kubectl create secret generic "${GROQ_SECRET_NAME}" \
  --from-literal=GROQ_API_KEY="${GROQ_API_KEY}" \
  --namespace "${NAMESPACE}" \
  --dry-run=client -o yaml | kubectl apply -f -

# --- Example app ---
echo "Applying example-app (deployment, service, servicemonitor)..."
kubectl apply -f "$(dirname "$0")/../k8s/example-app.yaml"

# --- Helm install/upgrade ---
echo "Installing/upgrading Helm release ${RELEASE_NAME}..."
helm upgrade --install "${RELEASE_NAME}" "${CHART_PATH}" \
  --namespace "${NAMESPACE}" \
  --set prometheus.url="${PROMETHEUS_URL}" \
  --set groq.existingSecret.name="${GROQ_SECRET_NAME}" \
  --set groq.existingSecret.key="GROQ_API_KEY" \
  --wait

echo "Done. Release status:"
helm status "${RELEASE_NAME}" --namespace "${NAMESPACE}"

# --- Port-forward Prometheus ---
echo "Starting port-forward for Prometheus on localhost:9090..."
kubectl port-forward svc/prometheus-kube-prometheus-prometheus 9090:9090 -n monitoring &
PF_PID=$!
echo "Prometheus available at http://localhost:9090 (pid: ${PF_PID})"
echo "To stop: kill ${PF_PID}"

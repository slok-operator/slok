{{/*
Expand the name of the chart.
*/}}
{{- define "slok.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "slok.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "slok.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "slok.labels" -}}
helm.sh/chart: {{ include "slok.chart" . }}
{{ include "slok.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "slok.selectorLabels" -}}
app.kubernetes.io/name: {{ include "slok.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
control-plane: controller-manager
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "slok.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (printf "%s-controller-manager" (include "slok.fullname" .)) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Webhook service name
*/}}
{{- define "slok.webhookServiceName" -}}
{{- printf "%s-webhook-service" (include "slok.fullname" .) }}
{{- end }}

{{/*
Certificate issuer name
*/}}
{{- define "slok.issuerName" -}}
{{- if .Values.certManager.issuer.name }}
{{- .Values.certManager.issuer.name }}
{{- else }}
{{- printf "%s-selfsigned-issuer" (include "slok.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Webhook certificate secret name
*/}}
{{- define "slok.webhookCertSecretName" -}}
{{- printf "%s-webhook-server-cert" (include "slok.fullname" .) }}
{{- end }}

{{/*
Dashboard backend name.
*/}}
{{- define "slok.dashboardBackendName" -}}
{{- printf "%s-dashboard-backend" (include "slok.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Dashboard frontend name.
*/}}
{{- define "slok.dashboardFrontendName" -}}
{{- printf "%s-dashboard-frontend" (include "slok.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Dashboard service account name.
*/}}
{{- define "slok.dashboardServiceAccountName" -}}
{{- if .Values.dashboard.serviceAccount.create }}
{{- default (printf "%s-dashboard" (include "slok.fullname" .)) .Values.dashboard.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.dashboard.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Dashboard labels.
*/}}
{{- define "slok.dashboardLabels" -}}
helm.sh/chart: {{ include "slok.chart" . }}
app.kubernetes.io/name: {{ include "slok.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Dashboard backend selector labels.
*/}}
{{- define "slok.dashboardBackendSelectorLabels" -}}
app.kubernetes.io/name: {{ include "slok.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: dashboard-backend
{{- end }}

{{/*
Dashboard frontend selector labels.
*/}}
{{- define "slok.dashboardFrontendSelectorLabels" -}}
app.kubernetes.io/name: {{ include "slok.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: dashboard-frontend
{{- end }}

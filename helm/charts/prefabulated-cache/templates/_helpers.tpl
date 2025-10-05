{{/*
Expand the name of the chart.
*/}}
{{- define "prefabulated-cache.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "prefabulated-cache.fullname" -}}
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
{{- define "prefabulated-cache.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "prefabulated-cache.labels" -}}
helm.sh/chart: {{ include "prefabulated-cache.chart" . }}
{{ include "prefabulated-cache.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: cloud-native-encabulator
app.kubernetes.io/component: data-plane
{{- end }}

{{/*
Selector labels
*/}}
{{- define "prefabulated-cache.selectorLabels" -}}
app.kubernetes.io/name: {{ include "prefabulated-cache.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

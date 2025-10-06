{{/*
Expand the name of the chart.
*/}}
{{- define "panametric-fam-processor.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "panametric-fam-processor.fullname" -}}
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
{{- define "panametric-fam-processor.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "panametric-fam-processor.labels" -}}
helm.sh/chart: {{ include "panametric-fam-processor.chart" . }}
{{ include "panametric-fam-processor.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: cloud-native-encabulator
app.kubernetes.io/component: processing-plane
{{- end }}

{{/*
Selector labels
*/}}
{{- define "panametric-fam-processor.selectorLabels" -}}
app.kubernetes.io/name: {{ include "panametric-fam-processor.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Shared helpers. Every service object in the chart is rendered from these, so a change
here changes all fifteen workloads at once (design section 10).
*/}}

{{/* ---------------------------------------------------------------- names ---- */}}

{{- define "encabulator.namespace" -}}
{{- .Release.Namespace -}}
{{- end -}}

{{- define "encabulator.chartLabel" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "encabulator.imageTag" -}}
{{- default .Chart.AppVersion .Values.global.imageTag -}}
{{- end -}}

{{/*
Image reference for a service.
args: dict "root" $ "name" <service> "svc" <service values>
*/}}
{{- define "encabulator.image" -}}
{{- $root := .root -}}
{{- $img := default (dict) (get .svc "image") -}}
{{- $repo := default (printf "%s/%s" $root.Values.global.registry .name) (get $img "repository") -}}
{{- $tag := default (include "encabulator.imageTag" $root) (get $img "tag") -}}
{{- if get $img "digest" -}}
{{- printf "%s@%s" $repo (get $img "digest") -}}
{{- else -}}
{{- printf "%s:%s" $repo $tag -}}
{{- end -}}
{{- end -}}

{{/* ---------------------------------------------------------------- labels ---- */}}

{{/*
Labels common to everything the chart renders (design section 10).
args: dict "root" $ "name" <object name> "plane" <plane> "component" <component>
*/}}
{{- define "encabulator.labels" -}}
app.kubernetes.io/name: {{ .name | quote }}
app.kubernetes.io/instance: {{ .root.Release.Name | quote }}
app.kubernetes.io/part-of: encabulator
app.kubernetes.io/version: {{ include "encabulator.imageTag" .root | quote }}
app.kubernetes.io/managed-by: {{ .root.Release.Service | quote }}
helm.sh/chart: {{ include "encabulator.chartLabel" .root | quote }}
{{- if .component }}
app.kubernetes.io/component: {{ .component | quote }}
{{- end }}
{{- if .plane }}
encabulator.io/plane: {{ .plane | quote }}
{{- end }}
{{- with .root.Values.global.labels }}
{{ toYaml . }}
{{- end }}
{{- end -}}

{{/* Selector labels: the stable subset. */}}
{{- define "encabulator.selectorLabels" -}}
app.kubernetes.io/name: {{ .name | quote }}
app.kubernetes.io/instance: {{ .root.Release.Name | quote }}
{{- end -}}

{{- define "encabulator.annotations" -}}
{{- with .Values.global.annotations }}
{{ toYaml . }}
{{- end }}
{{- end -}}

{{/* ------------------------------------------------------- service defaults ---- */}}

{{/*
Merge defaults into one service entry. Returns the merged dict.
args: dict "root" $ "svc" <service values>
Note: mergeOverwrite mutates, so deep-copy both sides first.
*/}}
{{- define "encabulator.svc" -}}
{{- $defaults := deepCopy .root.Values.defaults -}}
{{- $svc := deepCopy .svc -}}
{{- toYaml (mergeOverwrite $defaults $svc) -}}
{{- end -}}

{{/* Upper-snake environment variable prefix for a service name. */}}
{{- define "encabulator.envName" -}}
{{- . | upper | replace "-" "_" -}}
{{- end -}}

{{/* ------------------------------------------------------------- store hosts ---- */}}

{{- define "encabulator.postgres.host" -}}
{{- if .Values.postgres.external.enabled -}}
{{- required "postgres.external.host is required when postgres.external.enabled" .Values.postgres.external.host -}}
{{- else -}}
postgres
{{- end -}}
{{- end -}}

{{- define "encabulator.postgres.port" -}}
{{- if .Values.postgres.external.enabled -}}{{ .Values.postgres.external.port }}{{- else -}}{{ .Values.postgres.service.port }}{{- end -}}
{{- end -}}

{{- define "encabulator.postgres.sslmode" -}}
{{- if .Values.postgres.external.enabled -}}{{ .Values.postgres.external.sslmode }}{{- else -}}disable{{- end -}}
{{- end -}}

{{- define "encabulator.postgres.secretName" -}}
{{- if and .Values.postgres.external.enabled .Values.postgres.external.existingSecret -}}
{{- .Values.postgres.external.existingSecret -}}
{{- else -}}
{{ printf "%s-postgres" .Release.Name }}
{{- end -}}
{{- end -}}

{{- define "encabulator.valkey.host" -}}
{{- if .Values.valkey.external.enabled -}}
{{- required "valkey.external.host is required when valkey.external.enabled" .Values.valkey.external.host -}}
{{- else -}}
valkey
{{- end -}}
{{- end -}}

{{- define "encabulator.valkey.port" -}}
{{- if .Values.valkey.external.enabled -}}{{ .Values.valkey.external.port }}{{- else -}}{{ .Values.valkey.service.port }}{{- end -}}
{{- end -}}

{{- define "encabulator.valkey.scheme" -}}
{{- if and .Values.valkey.external.enabled .Values.valkey.external.tls -}}rediss{{- else -}}redis{{- end -}}
{{- end -}}

{{- define "encabulator.valkey.secretName" -}}
{{- if and .Values.valkey.external.enabled .Values.valkey.external.existingSecret -}}
{{- .Values.valkey.external.existingSecret -}}
{{- else -}}
{{ printf "%s-valkey" .Release.Name }}
{{- end -}}
{{- end -}}

{{- define "encabulator.nats.url" -}}
{{- if .Values.nats.external.enabled -}}
{{- required "nats.external.url is required when nats.external.enabled" .Values.nats.external.url -}}
{{- else -}}
{{- printf "nats://nats:%v" .Values.nats.service.port -}}
{{- end -}}
{{- end -}}

{{/*
The KEDA operator polls this endpoint from its own namespace, so the in-cluster Service name
must carry the release namespace: a bare "nats" would resolve to nats.<keda namespace>.
*/}}
{{- define "encabulator.nats.monitoringEndpoint" -}}
{{- if .Values.nats.external.enabled -}}
{{- required "nats.external.monitoringEndpoint is required when nats.external.enabled and keda.enabled" .Values.nats.external.monitoringEndpoint -}}
{{- else -}}
{{- printf "nats.%s.svc:%v" (include "encabulator.namespace" .) .Values.nats.service.monitoringPort -}}
{{- end -}}
{{- end -}}

{{- define "encabulator.influxdb.url" -}}
{{- if .Values.influxdb.external.enabled -}}
{{- required "influxdb.external.url is required when influxdb.external.enabled" .Values.influxdb.external.url -}}
{{- else -}}
{{- printf "http://influxdb:%v" .Values.influxdb.service.port -}}
{{- end -}}
{{- end -}}

{{- define "encabulator.influxdb.secretName" -}}
{{- if and .Values.influxdb.external.enabled .Values.influxdb.external.existingSecret -}}
{{- .Values.influxdb.external.existingSecret -}}
{{- else -}}
{{ printf "%s-influxdb" .Release.Name }}
{{- end -}}
{{- end -}}

{{- define "encabulator.influxdb.secretKey" -}}
{{- if and .Values.influxdb.external.enabled .Values.influxdb.external.existingSecret -}}
{{- .Values.influxdb.external.existingSecretKey -}}
{{- else -}}
{{- .Values.influxdb.auth.tokenKey -}}
{{- end -}}
{{- end -}}

{{- define "encabulator.appSecretName" -}}
{{- default (printf "%s-app" .Release.Name) .Values.secrets.existingSecret -}}
{{- end -}}

{{/*
Resolve a `secretEnv` reference to a Secret name.
args: dict "root" $ "secret" <alias>
*/}}
{{- define "encabulator.secretRefName" -}}
{{- $r := .root -}}
{{- if eq .secret "app" -}}{{ include "encabulator.appSecretName" $r }}
{{- else if eq .secret "postgres" -}}{{ include "encabulator.postgres.secretName" $r }}
{{- else if eq .secret "valkey" -}}{{ include "encabulator.valkey.secretName" $r }}
{{- else if eq .secret "influxdb" -}}{{ include "encabulator.influxdb.secretName" $r }}
{{- else -}}{{ .secret }}
{{- end -}}
{{- end -}}

{{/* ----------------------------------------------------------- secret values ---- */}}

{{/*
Return the base64 value for a Secret key, preserving whatever is already installed.
args: dict "root" $ "secret" <secret name> "key" <key> "value" <generated plaintext>
*/}}
{{- define "encabulator.preserved" -}}
{{- $root := .root -}}
{{- $existing := lookup "v1" "Secret" $root.Release.Namespace .secret -}}
{{- if and $existing $existing.data (index $existing.data .key) -}}
{{- index $existing.data .key -}}
{{- else -}}
{{- .value | b64enc -}}
{{- end -}}
{{- end -}}

{{/*
Plaintext variant: used where one generated value has to appear in two keys
(the InfluxDB token, which is stored raw and inside a JSON file).
*/}}
{{- define "encabulator.preservedPlain" -}}
{{- $root := .root -}}
{{- $existing := lookup "v1" "Secret" $root.Release.Namespace .secret -}}
{{- if and $existing $existing.data (index $existing.data .key) -}}
{{- index $existing.data .key | b64dec -}}
{{- else -}}
{{- .value -}}
{{- end -}}
{{- end -}}

{{/* ---------------------------------------------------------------- env list ---- */}}

{{/*
The full environment for one service, per Appendix A. Emitted in a deliberate order:
secret-backed password variables come first so the Kubernetes $(VAR) expansion used to
build DATABASE_URL and VALKEY_URL resolves.
args: dict "root" $ "name" <service name> "svc" <merged service values>
*/}}
{{- define "encabulator.env" -}}
{{- $root := .root -}}
{{- $name := .name -}}
{{- $svc := .svc -}}
{{- $stores := default (dict) $svc.stores -}}
- name: PORT
  value: {{ $svc.port | quote }}
- name: LOG_LEVEL
  value: {{ default $root.Values.logLevel $svc.logLevel | quote }}
- name: OTEL_SERVICE_NAME
  value: {{ $name | quote }}
- name: OTEL_RESOURCE_ATTRIBUTES
  value: {{ printf "service.namespace=encabulator,service.version=%s,encabulator.plane=%s" (include "encabulator.imageTag" $root) $svc.plane | quote }}
{{- if $root.Values.observability.otlpEndpoint }}
- name: OTEL_EXPORTER_OTLP_ENDPOINT
  value: {{ $root.Values.observability.otlpEndpoint | quote }}
- name: OTEL_EXPORTER_OTLP_PROTOCOL
  value: http/protobuf
{{- end }}
{{- if has $name $root.Values.wiring.malaiseEngineUrl }}
- name: MALAISE_ENGINE_URL
  value: "http://malaise-injection-engine:8080"
{{- end }}
{{- if has $name $root.Values.wiring.girdlespringUrl }}
- name: DIFFERENTIAL_GIRDLESPRING_URL
  value: "http://differential-girdlespring:8080"
{{- end }}
{{- range $callee := (default (list) (get $root.Values.wiring.serviceUrls $name)) }}
{{- if ne $callee "differential-girdlespring" }}
- name: {{ printf "%s_URL" (include "encabulator.envName" $callee) }}
  value: {{ printf "http://%s:8080" $callee | quote }}
{{- end }}
{{- end }}
{{- if has $name $root.Values.wiring.prometheusUrl }}
- name: PROMETHEUS_URL
  value: {{ $root.Values.observability.prometheusUrl | quote }}
{{- end }}
{{- if has $name $root.Values.wiring.podNamespace }}
- name: POD_NAMESPACE
  valueFrom:
    fieldRef:
      fieldPath: metadata.namespace
{{- end }}
- name: POD_NAME
  valueFrom:
    fieldRef:
      fieldPath: metadata.name
{{- if eq $name "encabulation-control-console" }}
- name: GRAFANA_EXTERNAL_URL
  value: {{ $root.Values.observability.grafanaExternalUrl | quote }}
- name: JAEGER_EXTERNAL_URL
  value: {{ $root.Values.observability.jaegerExternalUrl | quote }}
{{- end }}
{{- /* -------- backing stores -------- */}}
{{- if hasKey $stores "postgres" }}
{{- $db := required (printf "services.%s.stores.postgres.database is required" $name) (get (default (dict) (get $stores "postgres")) "database") -}}
{{- $dbcfg := required (printf "postgres.databases.%s is not defined" $db) (get $root.Values.postgres.databases $db) }}
- name: PGPASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "encabulator.postgres.secretName" $root | quote }}
      key: {{ $dbcfg.passwordKey | quote }}
- name: DATABASE_URL
  value: {{ printf "postgres://%s:$(PGPASSWORD)@%s:%v/%s?sslmode=%s" $dbcfg.user (include "encabulator.postgres.host" $root) (include "encabulator.postgres.port" $root) $db (include "encabulator.postgres.sslmode" $root) | quote }}
{{- end }}
{{- if hasKey $stores "valkey" }}
- name: VALKEY_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "encabulator.valkey.secretName" $root | quote }}
      key: {{ $root.Values.valkey.passwordKey | quote }}
- name: VALKEY_URL
  value: {{ printf "%s://%s:$(VALKEY_PASSWORD)@%s:%v/%v" (include "encabulator.valkey.scheme" $root) $root.Values.valkey.user (include "encabulator.valkey.host" $root) (include "encabulator.valkey.port" $root) $root.Values.valkey.database | quote }}
{{- end }}
{{- if hasKey $stores "nats" }}
- name: NATS_URL
  value: {{ include "encabulator.nats.url" $root | quote }}
{{- end }}
{{- if hasKey $stores "influxdb" }}
- name: INFLUXDB_URL
  value: {{ include "encabulator.influxdb.url" $root | quote }}
- name: INFLUXDB_DATABASE
  value: {{ $root.Values.influxdb.database | quote }}
{{- if or (not $root.Values.influxdb.external.enabled) $root.Values.influxdb.external.existingSecret }}
- name: INFLUXDB_TOKEN
  valueFrom:
    secretKeyRef:
      name: {{ include "encabulator.influxdb.secretName" $root | quote }}
      key: {{ include "encabulator.influxdb.secretKey" $root | quote }}
{{- end }}
{{- end }}
{{- /* -------- per-service secrets and plain env -------- */}}
{{- range $k, $ref := (default (dict) $svc.secretEnv) }}
- name: {{ $k }}
  valueFrom:
    secretKeyRef:
      name: {{ include "encabulator.secretRefName" (dict "root" $root "secret" $ref.secret) | quote }}
      key: {{ $ref.key | quote }}
{{- end }}
{{- range $k, $v := (default (dict) $svc.env) }}
- name: {{ $k }}
  value: {{ tpl (toString $v) $root | quote }}
{{- end }}
{{- range $k, $v := (default (dict) $svc.extraEnv) }}
- name: {{ $k }}
  value: {{ tpl (toString $v) $root | quote }}
{{- end }}
{{- end -}}

{{/* ---------------------------------------------------------------- probes ---- */}}

{{- define "encabulator.probes" -}}
{{- $svc := .svc -}}
{{- with $svc.probes.liveness }}
livenessProbe:
  httpGet:
    path: {{ .path }}
    port: http
  initialDelaySeconds: {{ .initialDelaySeconds }}
  periodSeconds: {{ .periodSeconds }}
  timeoutSeconds: {{ .timeoutSeconds }}
  failureThreshold: {{ .failureThreshold }}
{{- end }}
{{- with $svc.probes.readiness }}
readinessProbe:
  httpGet:
    path: {{ .path }}
    port: http
  initialDelaySeconds: {{ .initialDelaySeconds }}
  periodSeconds: {{ .periodSeconds }}
  timeoutSeconds: {{ .timeoutSeconds }}
  failureThreshold: {{ .failureThreshold }}
{{- end }}
{{- if $svc.probes.startup.enabled }}
startupProbe:
  httpGet:
    path: {{ $svc.probes.startup.path }}
    port: http
  periodSeconds: {{ $svc.probes.startup.periodSeconds }}
  failureThreshold: {{ $svc.probes.startup.failureThreshold }}
{{- end }}
{{- end -}}

{{/* ------------------------------------------------------------- autoscaling ---- */}}

{{/*
True when something other than the Deployment's own replicas field owns the replica
count, in which case `replicas` is omitted so Helm does not fight the autoscaler.
*/}}
{{- define "encabulator.isAutoscaled" -}}
{{- $root := .root -}}
{{- if and $root.Values.keda.enabled (eq .name "logarithmic-casing-line") -}}
true
{{- else if .svc.autoscaling.enabled -}}
true
{{- else -}}
{{- end -}}
{{- end -}}

{{/* True when the workload may run more than one replica, so a PDB makes sense. */}}
{{- define "encabulator.wantsPDB" -}}
{{- $svc := .svc -}}
{{- if not $svc.podDisruptionBudget.enabled -}}
{{- else if include "encabulator.isAutoscaled" . -}}
true
{{- else if gt (int $svc.replicas) 1 -}}
true
{{- else -}}
{{- end -}}
{{- end -}}

{{/* --------------------------------------------------------------- store bits ---- */}}

{{/*
Labels for a backing store StatefulSet.
args: dict "root" $ "name" <store name>
*/}}
{{- define "encabulator.storeLabels" -}}
{{- include "encabulator.labels" (dict "root" .root "name" .name "plane" "data" "component" "backing-store") -}}
{{- end -}}

{{/*
Labels for a StatefulSet's volumeClaimTemplates. volumeClaimTemplates cannot change after the
StatefulSet exists, so these leave out everything that changes with a release
(helm.sh/chart, app.kubernetes.io/version); with them, every chart or image bump failed the
upgrade with "spec.volumeClaimTemplates: field is immutable".
*/}}
{{- define "encabulator.storeVolumeLabels" -}}
{{- include "encabulator.selectorLabels" (dict "root" .root "name" .name) }}
app.kubernetes.io/part-of: encabulator
app.kubernetes.io/component: backing-store
encabulator.io/plane: data
{{- end -}}

{{- define "encabulator.storePodLabels" -}}
{{- include "encabulator.selectorLabels" (dict "root" .root "name" .name) }}
app.kubernetes.io/part-of: encabulator
app.kubernetes.io/component: backing-store
encabulator.io/plane: data
{{- if .root.Values.stores.malaiseTarget }}
encabulator.io/malaise-target: "true"
{{- end }}
{{- end -}}

{{/*
A store's storageClassName wins, then storageClass.name when the chart creates
one, then global.storageClassName; empty means the cluster default.
*/}}
{{- define "encabulator.pvc" -}}
{{- $p := .persistence -}}
{{- $v := .root.Values -}}
{{- $class := $p.storageClassName | default (ternary $v.storageClass.name "" $v.storageClass.create) | default $v.global.storageClassName -}}
accessModes: {{ toYaml $p.accessModes | nindent 2 }}
resources:
  requests:
    storage: {{ $p.size | quote }}
{{- if $class }}
storageClassName: {{ $class | quote }}
{{- end }}
{{- end -}}

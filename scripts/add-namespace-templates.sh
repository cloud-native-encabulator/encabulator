#!/bin/sh
# Add namespace.yaml templates to all Helm charts

set -e

echo "Adding namespace templates to Helm charts..."

# Function to get namespace for a chart
get_namespace() {
  case "$1" in
    cardinal-grammeter|demo-scenario-orchestrator|encabulation-control-console|malaise-injection-engine)
      echo "encabulator-control" ;;
    hydrocoptic-datastore|prefabulated-cache|sinusoidal-database)
      echo "encabulator-data" ;;
    dingle-arm-api-gateway|logarithmic-casing-service|panametric-fam-processor)
      echo "encabulator-processing" ;;
    differential-girdle|side-fumbling-collector|unilateral-phase-detractor)
      echo "encabulator-support" ;;
    *)
      echo "unknown" ;;
  esac
}

# Process each chart
for chart in cardinal-grammeter demo-scenario-orchestrator encabulation-control-console malaise-injection-engine \
             hydrocoptic-datastore prefabulated-cache sinusoidal-database \
             dingle-arm-api-gateway logarithmic-casing-service panametric-fam-processor \
             differential-girdle side-fumbling-collector unilateral-phase-detractor; do
  
  namespace=$(get_namespace "$chart")
  template_dir="helm/charts/$chart/templates"
  
  if [ ! -d "$template_dir" ]; then
    echo "⚠️  Skipping $chart - templates directory not found"
    continue
  fi
  
  echo "Adding namespace template to $chart (namespace: $namespace)..."
  
  cat > "$template_dir/namespace.yaml" <<EOF
{{- if .Values.createNamespace }}
apiVersion: v1
kind: Namespace
metadata:
  name: {{ .Values.namespace | default "$namespace" }}
  labels:
    name: {{ .Values.namespace | default "$namespace" }}
    {{- include "${chart}.labels" . | nindent 4 }}
{{- end }}
EOF
  
  echo "  ✓ Created $template_dir/namespace.yaml"
done

echo ""
echo "✓ Namespace templates added to all charts"
echo ""
echo "Note: Charts will create namespaces when createNamespace=true (default: false)"

#!/bin/sh
# Add namespace configuration to values.yaml files

set -e

echo "Adding namespace configuration to values.yaml files..."

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
  values_file="helm/charts/$chart/values.yaml"
  
  if [ ! -f "$values_file" ]; then
    echo "⚠️  Skipping $chart - values.yaml not found"
    continue
  fi
  
  # Check if namespace config already exists
  if grep -q "^namespace:" "$values_file" 2>/dev/null; then
    echo "  ⏭️  $chart already has namespace config"
    continue
  fi
  
  echo "Adding namespace config to $chart (namespace: $namespace)..."
  
  # Add namespace config at the top of values.yaml
  temp_file=$(mktemp)
  cat > "$temp_file" <<EOF
# Namespace configuration
namespace: $namespace
createNamespace: false

EOF
  cat "$values_file" >> "$temp_file"
  mv "$temp_file" "$values_file"
  
  echo "  ✓ Updated $values_file"
done

echo ""
echo "✓ Namespace configuration added to all values.yaml files"

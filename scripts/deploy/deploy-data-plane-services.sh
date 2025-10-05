#!/bin/bash

set -e

NAMESPACE="${1:-encabulator-data}"

echo "Deploying Data Plane Services to namespace: $NAMESPACE..."

# Ensure namespace exists
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

SERVICES=(
    "hydrocoptic-datastore"
    "prefabulated-cache"
    "sinusoidal-database"
)

for service in "${SERVICES[@]}"; do
    echo "Deploying $service..."
    helm upgrade --install "$service" \
        "./helm/charts/$service" \
        --namespace "$NAMESPACE" \
        --create-namespace \
        --wait
    echo "$service deployed successfully!"
    echo ""
done

echo "All Data Plane services deployed successfully!"
echo ""
echo "Check status with:"
echo "  kubectl get pods -n $NAMESPACE"
echo "  kubectl get svc -n $NAMESPACE"

#!/bin/bash

set -e

echo "Building Data Plane Docker Images..."

SERVICES=(
    "hydrocoptic-datastore"
    "prefabulated-cache"
    "sinusoidal-database"
)

TAG="${1:-latest}"

for service in "${SERVICES[@]}"; do
    echo "Building Docker image for $service..."
    docker build -t "$service:$TAG" "services/data-plane/$service"
    echo "$service image built successfully!"
    echo ""
done

echo "All Data Plane Docker images built successfully!"
echo ""
echo "Images:"
for service in "${SERVICES[@]}"; do
    echo "  - $service:$TAG"
done

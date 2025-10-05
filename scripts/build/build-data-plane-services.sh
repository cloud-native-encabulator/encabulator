#!/bin/bash

set -e

echo "Building Data Plane Services..."

SERVICES=(
    "hydrocoptic-datastore"
    "prefabulated-cache"
    "sinusoidal-database"
)

for service in "${SERVICES[@]}"; do
    echo "Building $service..."
    cd "services/data-plane/$service"
    
    # Run go mod tidy to generate go.sum
    echo "Running go mod tidy for $service..."
    go mod tidy
    
    # Build the service
    echo "Building binary for $service..."
    go build -o "$service" ./cmd/server
    
    # Run tests
    echo "Running tests for $service..."
    go test ./...
    
    cd ../../..
    echo "$service build complete!"
    echo ""
done

echo "All Data Plane services built successfully!"

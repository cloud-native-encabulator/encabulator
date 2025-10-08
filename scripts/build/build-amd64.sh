#!/bin/bash

# Build images for AMD64 architecture (for EKS x86_64 nodes)

set -e

echo "Building images for AMD64 architecture..."

# Build all images for AMD64
finch build --platform linux/amd64 -t hydrocoptic-datastore:latest services/data-plane/hydrocoptic-datastore
finch build --platform linux/amd64 -t prefabulated-cache:latest services/data-plane/prefabulated-cache
finch build --platform linux/amd64 -t sinusoidal-database:latest services/data-plane/sinusoidal-database

echo "✓ AMD64 images built successfully"
echo ""
echo "Now publish with: make image-publish IMAGE_TAG=0.0.1"

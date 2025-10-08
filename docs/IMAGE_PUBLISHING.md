# Container Image Publishing Guide

This guide explains how to build and publish container images to Amazon ECR using the Makefile targets.

## Prerequisites

1. AWS CLI configured with appropriate credentials
2. ECR repositories created for each service
3. ECR login completed:
   ```bash
   aws ecr get-login-password --region us-east-2 | \
     finch login --username AWS --password-stdin \
     510431938379.dkr.ecr.us-east-2.amazonaws.com
   ```

## Configuration

The following environment variables can be overridden:

- `ECR_REGISTRY` - ECR registry URL (default: `510431938379.dkr.ecr.us-east-2.amazonaws.com`)
- `ECR_NAMESPACE` - ECR namespace/prefix (default: `encabulator`)
- `IMAGE_TAG` - Image tag (default: `latest`)
- `CONTAINER_RUNTIME` - Container runtime to use (default: `finch`, can be `docker`)

## Publishing Images

### Publish All Images

Build and publish all service images to ECR:

```bash
make image-publish
```

### Publish by Plane

Publish images for a specific plane:

```bash
# Data Plane services
make image-publish-data-plane

# Processing Plane services
make image-publish-processing-plane

# Support Plane services
make image-publish-support-plane

# Control Plane services
make image-publish-control-plane
```

### Publish Individual Service

Navigate to a service directory and publish:

```bash
cd services/data-plane/hydrocoptic-datastore
make image-publish
```

## Custom Configuration

### Using a Different Tag

```bash
make image-publish IMAGE_TAG=v1.2.3
```

### Using Docker Instead of Finch

```bash
make image-publish CONTAINER_RUNTIME=docker
```

### Using a Different Registry

```bash
make image-publish \
  ECR_REGISTRY=123456789012.dkr.ecr.us-west-2.amazonaws.com \
  ECR_NAMESPACE=my-namespace \
  IMAGE_TAG=production
```

## How It Works

The `image-publish` target:

1. Builds the container image locally (if not already built)
2. Tags the image with the ECR repository path
3. Pushes the image to ECR

The target automatically handles:
- Building images before publishing
- Proper ECR image naming
- Error handling and exit codes

## Service Images

The following images will be published:

### Data Plane
- `encabulator/hydrocoptic-datastore`
- `encabulator/prefabulated-cache`
- `encabulator/sinusoidal-database`

### Processing Plane
- `encabulator/panametric-fam-processor`
- `encabulator/logarithmic-casing-service`
- `encabulator/dingle-arm-api-gateway`

### Support Plane
- `encabulator/unilateral-phase-detractor`
- `encabulator/differential-girdle`
- `encabulator/side-fumbling-collector`

### Control Plane
- `encabulator/encabulation-control-console`
- `encabulator/malaise-injection-engine`
- `encabulator/cardinal-grammeter`
- `encabulator/demo-scenario-orchestrator`

## Troubleshooting

### Authentication Errors

If you see authentication errors, re-run the ECR login:

```bash
aws ecr get-login-password --region us-east-2 | \
  finch login --username AWS --password-stdin \
  510431938379.dkr.ecr.us-east-2.amazonaws.com
```

### Repository Not Found

Ensure ECR repositories exist:

```bash
# List repositories
aws ecr describe-repositories --region us-east-2 | grep repositoryName

# Create a repository if needed
aws ecr create-repository \
  --repository-name encabulator/service-name \
  --region us-east-2
```

### Build Failures

If image builds fail, try building without publishing first:

```bash
make image
```

Then publish separately:

```bash
make image-publish
```

## CI/CD Integration

For automated builds, you can use:

```bash
# Build and publish with version tag
VERSION=$(git describe --tags --always)
make image-publish IMAGE_TAG=$VERSION

# Also tag as latest
make image-publish IMAGE_TAG=latest
```

## Next Steps

After publishing images:

1. Update Helm chart values to reference ECR images
2. Deploy services using Helm
3. Verify deployments in Kubernetes

See [HELM_DEPLOYMENT.md](./HELM_DEPLOYMENT.md) for deployment instructions.

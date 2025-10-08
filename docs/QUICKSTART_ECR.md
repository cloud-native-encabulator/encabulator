# Quick Start: Publishing Images to ECR

This guide will get you publishing container images to Amazon ECR in under 5 minutes.

## Prerequisites

- AWS CLI installed and configured
- Finch or Docker installed
- ECR repositories created (see setup below)

## Step 1: ECR Login

```bash
# Login to ECR
aws ecr get-login-password --region us-east-2 | \
  finch login --username AWS --password-stdin \
  510431938379.dkr.ecr.us-east-2.amazonaws.com
```

If using Docker instead of Finch:
```bash
aws ecr get-login-password --region us-east-2 | \
  docker login --username AWS --password-stdin \
  510431938379.dkr.ecr.us-east-2.amazonaws.com
```

## Step 2: Verify ECR Repositories

Check that repositories exist:
```bash
aws ecr describe-repositories --region us-east-2 | grep repositoryName
```

If repositories don't exist, create them using the setup script:
```bash
./scripts/setup/create-ecr-repos.sh
```

## Step 3: Publish Images

### Option A: Publish All Images
```bash
make image-publish
```

This will:
1. Build all 13 service images (if not already built)
2. Tag them with ECR repository paths
3. Push them to ECR

### Option B: Publish by Plane
```bash
# Just data plane services
make image-publish-data-plane

# Just processing plane services
make image-publish-processing-plane

# Just support plane services
make image-publish-support-plane

# Just control plane services
make image-publish-control-plane
```

### Option C: Publish Single Service
```bash
cd services/data-plane/hydrocoptic-datastore
make image-publish
```

## Step 4: Verify Images in ECR

```bash
# List images for a specific service
aws ecr list-images \
  --repository-name encabulator/hydrocoptic-datastore \
  --region us-east-2

# Or check all repositories
for repo in $(aws ecr describe-repositories --region us-east-2 --query 'repositories[?starts_with(repositoryName, `encabulator/`)].repositoryName' --output text); do
  echo "Repository: $repo"
  aws ecr list-images --repository-name $repo --region us-east-2 --query 'imageIds[*].imageTag' --output text
  echo ""
done
```

## Common Options

### Use a Specific Tag
```bash
make image-publish IMAGE_TAG=v1.0.0
```

### Use Docker Instead of Finch
```bash
make image-publish CONTAINER_RUNTIME=docker
```

### Use a Different Registry
```bash
make image-publish \
  ECR_REGISTRY=123456789012.dkr.ecr.us-west-2.amazonaws.com \
  ECR_NAMESPACE=my-namespace
```

## Troubleshooting

### "no basic auth credentials"
Your ECR login has expired. Re-run the login command from Step 1.

### "repository does not exist"
Create the ECR repositories:
```bash
./scripts/setup/create-ecr-repos.sh
```

### Build failures
Try building without publishing first:
```bash
make image
```

Then publish:
```bash
make image-publish
```

## What's Next?

After publishing images to ECR:

1. **Update Helm Charts**: Modify chart values to use ECR images
   ```bash
   # Edit helm/charts/*/values.yaml
   # Change image.repository to ECR paths
   ```

2. **Configure Image Pull Secrets**: Set up Kubernetes authentication
   ```bash
   kubectl create secret docker-registry ecr-secret \
     --docker-server=510431938379.dkr.ecr.us-east-2.amazonaws.com \
     --docker-username=AWS \
     --docker-password=$(aws ecr get-login-password --region us-east-2)
   ```

3. **Deploy to Kubernetes**: Use the updated Helm charts
   ```bash
   ./scripts/deploy/deploy-helm.sh
   ```

## Full Workflow Example

```bash
# 1. Login to ECR
aws ecr get-login-password --region us-east-2 | \
  finch login --username AWS --password-stdin \
  510431938379.dkr.ecr.us-east-2.amazonaws.com

# 2. Build and publish all images with version tag
VERSION=v1.0.0
make image-publish IMAGE_TAG=$VERSION

# 3. Also tag as latest
make image-publish IMAGE_TAG=latest

# 4. Verify
aws ecr list-images \
  --repository-name encabulator/hydrocoptic-datastore \
  --region us-east-2

# 5. Update Helm charts and deploy
# (See Helm deployment documentation)
```

## CI/CD Integration

For automated pipelines:

```bash
#!/bin/bash
set -e

# Get version from git
VERSION=$(git describe --tags --always)

# Login to ECR
aws ecr get-login-password --region us-east-2 | \
  finch login --username AWS --password-stdin \
  510431938379.dkr.ecr.us-east-2.amazonaws.com

# Build and publish with version tag
make image-publish IMAGE_TAG=$VERSION

# Also tag as latest if on main branch
if [ "$BRANCH" = "main" ]; then
  make image-publish IMAGE_TAG=latest
fi
```

## Need More Help?

- [Full Image Publishing Guide](./IMAGE_PUBLISHING.md)
- [Makefile Reference](./MAKEFILE_REFERENCE.md)
- [ECR Setup Script](../scripts/setup/create-ecr-repos.sh)

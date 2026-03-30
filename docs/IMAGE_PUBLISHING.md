# Image Publishing

Build and publish container images to Amazon ECR.

## Prerequisites

- AWS CLI configured with appropriate credentials
- Finch or Docker installed
- ECR repositories created (`./scripts/setup/create-ecr-repos.sh`)

## ECR Login

```bash
aws ecr get-login-password --region us-east-2 | \
  finch login --username AWS --password-stdin \
  510431938379.dkr.ecr.us-east-2.amazonaws.com
```

For Docker, replace `finch` with `docker`.

## Publishing Images

```bash
# All images
make image-publish

# By plane
make image-publish-data-plane
make image-publish-processing-plane
make image-publish-support-plane
make image-publish-control-plane

# Single service
cd services/data-plane/hydrocoptic-datastore && make image-publish
```

## Configuration

Override via environment variables:

| Variable | Default |
|----------|---------|
| `ECR_REGISTRY` | `510431938379.dkr.ecr.us-east-2.amazonaws.com` |
| `ECR_NAMESPACE` | `encabulator` |
| `IMAGE_TAG` | `latest` |
| `CONTAINER_RUNTIME` | `finch` |

```bash
make image-publish IMAGE_TAG=v1.0.0
make image-publish CONTAINER_RUNTIME=docker
```

## Verify Published Images

```bash
aws ecr list-images \
  --repository-name encabulator/hydrocoptic-datastore \
  --region us-east-2
```

## Troubleshooting

- **"no basic auth credentials"** — ECR login expired. Re-run the login command.
- **"repository does not exist"** — Run `./scripts/setup/create-ecr-repos.sh`.
- **Build failures** — Try `make image` first, then `make image-publish`.

## Next Steps

After publishing:
1. Update Helm chart values to reference ECR images
2. Deploy with `./scripts/deploy/deploy-helm.sh` or via ArgoCD

See also: [Versioning](./VERSIONING.md) · [Makefile Reference](./MAKEFILE_REFERENCE.md)

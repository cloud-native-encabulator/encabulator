# ArgoCD Applications Directory

This directory contains the ArgoCD Application definitions that manage the entire Cloud Native Encabulator system through GitOps. These applications are deployed by the root applications defined in `k8s/platform/argocd/argocd-applications.yaml`.

## Directory Structure

```
argocd/
├── infrastructure/     # Infrastructure components (ACK, AWS resources, KRO)
├── platform/          # Platform services (monitoring, networking, namespaces)
├── applications/      # Cloud Native Encabulator microservices
└── README.md          # This file
```

## GitOps Workflow

These ArgoCD Applications are managed by root applications:

1. **infrastructure-root** → Manages `argocd/infrastructure/`
2. **platform-root** → Manages `argocd/platform/`
3. **applications-root** → Manages `argocd/applications/`

## Deployment Order

Applications deploy in dependency order:

1. **Infrastructure** (ACK controllers, AWS resources)
2. **Platform** (monitoring, networking, namespaces)
3. **Applications** (microservices)

## Repository Configuration

**Important**: Before deploying, update repository URLs using:

```bash
./scripts/setup/configure-repository.sh
```

This replaces `REPLACE_WITH_YOUR_ORG` placeholders with your actual repository.

## Usage

### Deploy All Applications (Recommended)
```bash
# Bootstrap complete GitOps workflow
./scripts/platform/bootstrap-gitops.sh
```

### Deploy Individual Categories
```bash
# Deploy infrastructure only
kubectl apply -k argocd/infrastructure/

# Deploy platform services
kubectl apply -k argocd/platform/

# Deploy applications
kubectl apply -k argocd/applications/
```

### Monitor Deployment
```bash
# Access ArgoCD UI
kubectl port-forward svc/argocd-server -n argocd 8080:443

# Check application status
kubectl get applications -n argocd
```

## Customization

Each directory contains:
- **Individual Application manifests** (`.yaml` files)
- **Kustomization file** for grouping and labeling
- **README.md** with specific documentation

See individual directory README files for detailed information about each component category.
# Infrastructure ArgoCD Applications

This directory contains ArgoCD Applications that manage the infrastructure layer of the Cloud Native Encabulator system.

## Components

### `ack-controllers.yaml`
**Purpose**: AWS Controllers for Kubernetes (ACK) installation  
**What it manages**: 
- RDS Controller (for PostgreSQL databases)
- ElastiCache Controller (for Redis clusters)
- IAM Controller (for AWS roles and policies)

**Installation Method**: OCI Helm charts from `oci://public.ecr.aws/aws-controllers-k8s`

**Target Namespace**: `ack-system`

### `aws-resources.yaml`
**Purpose**: AWS resources managed via ACK Custom Resources  
**What it manages**:
- Hydrocoptic Data Store (PostgreSQL via DBInstance CRD)
- Prefabulated Cache (Redis via ReplicationGroup CRD)
- IAM Roles (via Role CRD)

**Source**: Git repository `k8s/infrastructure/aws-resources/`

**Target Namespace**: `encabulator-data`

### `resource-orchestration.yaml`
**Purpose**: Kube Resource Orchestrator (KRO) for composite resources  
**What it manages**:
- KRO installation and configuration
- Custom resource composition definitions
- Complex multi-resource deployments

**Status**: Placeholder (to be implemented in Task 2.2)

**Target Namespace**: `kro-system`

## Dependencies

Infrastructure applications have the following dependency order:

1. **ACK Controllers** (must be installed first)
2. **AWS Resources** (depends on ACK controllers)
3. **Resource Orchestration** (depends on ACK and AWS resources)

## Configuration

### Prerequisites

Before deploying infrastructure applications:

1. **Configure Repository URL**:
   ```bash
   ./scripts/setup/configure-repository.sh
   ```

2. **Update AWS Configuration**:
   - Replace `ACCOUNT_ID` placeholders with your AWS account ID
   - Update IAM role ARNs in ACK controller configurations
   - Configure security groups and subnet groups in AWS resources

3. **Create IAM Roles**:
   - `ack-rds-controller` role with RDS permissions
   - `ack-elasticache-controller` role with ElastiCache permissions
   - `ack-iam-controller` role with IAM permissions

### Deployment

#### Via GitOps (Recommended)
```bash
# Deploy all infrastructure via root application
./scripts/platform/bootstrap-gitops.sh
```

#### Manual Deployment
```bash
# Deploy infrastructure applications
kubectl apply -k argocd/infrastructure/

# Monitor deployment
kubectl get applications -n argocd
```

#### Individual Components
```bash
# Deploy ACK controllers only
kubectl apply -f argocd/infrastructure/ack-controllers.yaml

# Deploy AWS resources only
kubectl apply -f argocd/infrastructure/aws-resources.yaml
```

## Monitoring

### Check Application Status
```bash
# List infrastructure applications
kubectl get applications -n argocd -l app.kubernetes.io/component=infrastructure

# Check specific application
kubectl describe application ack-rds-controller -n argocd
```

### Check Infrastructure Resources
```bash
# ACK controllers
kubectl get pods -n ack-system

# AWS resources
kubectl get dbinstance -n encabulator-data
kubectl get replicationgroup -n encabulator-data
kubectl get role -n ack-system
```

## Troubleshooting

### Common Issues

1. **ACK Controllers not starting**:
   - Check IAM role annotations and permissions
   - Verify AWS credentials configuration
   - Check cluster resources and node capacity

2. **AWS Resources not creating**:
   - Verify ACK controllers are running
   - Check AWS permissions and quotas
   - Validate security groups and networking

3. **ArgoCD sync failures**:
   - Check repository access and credentials
   - Verify Kustomize builds: `kustomize build argocd/infrastructure/`
   - Check ArgoCD logs: `kubectl logs -n argocd deployment/argocd-application-controller`

### Validation
```bash
# Run infrastructure validation
./scripts/infrastructure/validate-ack-setup.sh

# Test manifest syntax
./scripts/infrastructure/test-manifests.sh
```

## Security Considerations

- **IAM Roles**: Follow principle of least privilege
- **Network Security**: Configure appropriate security groups
- **Encryption**: Enable encryption at rest and in transit for AWS resources
- **Secrets**: Use Kubernetes Secrets for sensitive data

## Next Steps

After infrastructure deployment:

1. **Verify all components are healthy**
2. **Deploy platform services** (`argocd/platform/`)
3. **Deploy applications** (`argocd/applications/`)
4. **Configure monitoring and alerting**
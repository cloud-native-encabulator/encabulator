# Cloud Native Encabulator Infrastructure

This directory contains the Kubernetes manifests for managing AWS infrastructure using AWS Controllers for Kubernetes (ACK) and ArgoCD.

## Structure

```
k8s/infrastructure/
├── ack-controllers/          # ACK controller installations
│   ├── kustomization.yaml
│   ├── namespace.yaml
│   ├── rds-controller.yaml
│   ├── elasticache-controller.yaml
│   └── iam-controller.yaml
├── aws-resources/            # AWS resources via ACK CRDs
│   ├── kustomization.yaml
│   ├── hydrocoptic-datastore.yaml
│   ├── prefabulated-cache.yaml
│   └── iam-roles.yaml
├── config/                   # Environment-specific configuration
│   └── development.yaml
└── README.md
```

## AWS Resources Managed

### Data Plane Resources
- **Hydrocoptic Data Store**: PostgreSQL database via ACK RDS controller
- **Prefabulated Cache**: Redis cluster via ACK ElastiCache controller

### IAM Resources
- Service roles for data plane and processing plane components
- Policies for accessing AWS services
- Role policy attachments

## Prerequisites

1. **EKS Cluster** with OIDC provider configured
2. **ArgoCD** installed and configured
3. **AWS IAM roles** for ACK controllers with appropriate permissions
4. **VPC and Subnets** configured for RDS and ElastiCache

## Setup Instructions

### 1. Configure AWS Account Information

Edit `config/development.yaml` and replace placeholder values:

```yaml
aws:
  accountId: "123456789012"  # Your AWS account ID
  oidcProvider: "oidc.eks.us-west-2.amazonaws.com/id/EXAMPLED539D4633E53DE1B716D3041E"
```

### 2. Create IAM Roles for ACK Controllers

Create IAM roles for each ACK controller with the following trust policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::ACCOUNT_ID:oidc-provider/OIDC_PROVIDER"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "OIDC_PROVIDER:sub": "system:serviceaccount:ack-system:ack-CONTROLLER-controller",
          "OIDC_PROVIDER:aud": "sts.amazonaws.com"
        }
      }
    }
  ]
}
```

Required roles:
- `ack-rds-controller` with RDS permissions
- `ack-elasticache-controller` with ElastiCache permissions  
- `ack-iam-controller` with IAM permissions

### 3. Update Security Groups and Subnets

Replace placeholder values in the AWS resource manifests:
- Security group IDs for database and cache
- Subnet group names for RDS and ElastiCache

### 4. Choose Installation Method

#### Option A: GitOps with ArgoCD (Recommended)

Deploy the complete system using GitOps:

```bash
# 1. Configure your repository URL
./scripts/setup/configure-repository.sh

# 2. Bootstrap complete GitOps workflow
./scripts/platform/bootstrap-gitops.sh

# 3. Monitor deployment in ArgoCD UI
kubectl port-forward svc/argocd-server -n argocd 8080:443
```

#### Option B: Direct Kubernetes Manifests

Deploy using direct Kubernetes manifests (for development/testing):

```bash
# Complete installation (CRDs + Controllers)
./scripts/infrastructure/install-ack-complete.sh

# Or step by step:
./scripts/infrastructure/install-ack-crds.sh
kubectl apply -k k8s/infrastructure/ack-controllers/
```

#### Option C: Direct Helm Installation

Install using Helm directly (for custom setups):

```bash
# Install CRDs first
./scripts/infrastructure/pre-install-ack.sh

# Install controllers via Helm
helm install ack-rds-controller oci://public.ecr.aws/aws-controllers-k8s/rds-chart --version 1.2.11 -n ack-system --create-namespace
helm install ack-elasticache-controller oci://public.ecr.aws/aws-controllers-k8s/elasticache-chart --version 1.0.14 -n ack-system
helm install ack-iam-controller oci://public.ecr.aws/aws-controllers-k8s/iam-chart --version 1.3.8 -n ack-system
```

## Deployment Order

1. ACK Controllers are installed first
2. AWS Resources are created after controllers are ready
3. Applications can then reference the created AWS resources

## Monitoring

All ACK controllers expose Prometheus metrics on port 8080. The metrics include:
- Controller reconciliation status
- AWS API call metrics
- Resource creation/update/deletion events

## Troubleshooting

### Common Issues

1. **Controller pods not starting**: Check IAM role annotations and permissions
2. **Resources not being created**: Verify AWS credentials and regional availability
3. **Permission denied errors**: Ensure IAM roles have required AWS service permissions

### Useful Commands

```bash
# Check ACK controller status
kubectl get pods -n ack-system

# View controller logs
kubectl logs -n ack-system deployment/ack-rds-controller

# Check AWS resource status
kubectl get dbinstance -n encabulator-data
kubectl get replicationgroup -n encabulator-data

# Describe resources for troubleshooting
kubectl describe dbinstance hydrocoptic-datastore -n encabulator-data
```

## Security Considerations

- All database connections use encryption in transit and at rest
- Credentials are stored in Kubernetes Secrets
- IAM roles follow principle of least privilege
- Network access is restricted via security groups

## Environment Promotion

To promote to staging/production:
1. Create environment-specific configuration files
2. Update Kustomize overlays for environment differences
3. Create separate ArgoCD applications for each environment
4. Use different AWS accounts or regions for isolation
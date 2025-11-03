# AWS Controllers for Kubernetes (ACK) Setup Guide

This guide covers the setup and configuration of AWS Controllers for Kubernetes (ACK) for the Cloud Native Encabulator platform.

## Overview

ACK allows you to manage AWS resources directly from Kubernetes using Custom Resource Definitions (CRDs). This enables full GitOps workflows for infrastructure provisioning.

**Controllers Used:**
- **ACK IAM Controller**: Manages IAM roles and policies
- **ACK RDS Controller**: Manages RDS PostgreSQL databases
- **ACK ElastiCache Controller**: Manages ElastiCache Redis clusters

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     EKS Cluster                              │
│                                                              │
│  ┌────────────────────────────────────────────────────┐    │
│  │  ack-system namespace                               │    │
│  │                                                      │    │
│  │  ┌──────────────────┐  ┌──────────────────┐       │    │
│  │  │ ACK IAM          │  │ ACK RDS          │       │    │
│  │  │ Controller       │  │ Controller       │       │    │
│  │  │                  │  │                  │       │    │
│  │  │ ServiceAccount   │  │ ServiceAccount   │       │    │
│  │  │ + IAM Role       │  │ + IAM Role       │       │    │
│  │  └────────┬─────────┘  └────────┬─────────┘       │    │
│  │           │                     │                  │    │
│  │  ┌────────┴─────────────────────┴─────────┐       │    │
│  │  │ ACK ElastiCache Controller             │       │    │
│  │  │                                         │       │    │
│  │  │ ServiceAccount + IAM Role              │       │    │
│  │  └────────┬────────────────────────────────┘       │    │
│  └───────────┼──────────────────────────────────────┘    │
│              │                                            │
└──────────────┼────────────────────────────────────────────┘
               │
               │ AWS API Calls
               ▼
┌─────────────────────────────────────────────────────────────┐
│                        AWS                                   │
│                                                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────────┐             │
│  │   IAM    │  │   RDS    │  │ ElastiCache  │             │
│  │  Roles   │  │ Database │  │    Redis     │             │
│  └──────────┘  └──────────┘  └──────────────┘             │
└─────────────────────────────────────────────────────────────┘
```

## Prerequisites

- EKS cluster with OIDC provider enabled
- AWS CLI configured with appropriate credentials
- kubectl configured to access the cluster
- Cluster admin permissions

## Installation Steps

### Step 1: Bootstrap ACK IAM Controller

The ACK IAM controller needs an admin IAM role to manage other IAM roles. This is a one-time manual step.

```bash
# Run the bootstrap script
./scripts/setup/bootstrap-ack-iam.sh

# Or with custom parameters
CLUSTER_NAME=my-cluster AWS_REGION=us-west-2 ./scripts/setup/bootstrap-ack-iam.sh
```

**What this does:**
1. Creates `ack-system` namespace
2. Creates IAM policy with IAM management permissions
3. Creates IAM role `ack-iam-controller-admin-role` with OIDC trust policy
4. Creates Kubernetes service account with IAM role annotation

**Output:**
```
[INFO] ACK IAM Controller Bootstrap Complete!
Configuration:
  Cluster:         my-cluster
  Region:          us-east-2
  Namespace:       ack-system
  Service Account: ack-iam-controller
  IAM Role:        ack-iam-controller-admin-role
  Role ARN:        arn:aws:iam::123456789012:role/ack-iam-controller-admin-role
```

### Step 2: Install ACK IAM Controller via ArgoCD

Create ArgoCD Application for ACK IAM controller:

```yaml
# argocd/apps/infrastructure/ack-iam-controller.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: ack-iam-controller
  namespace: argocd
spec:
  project: default
  source:
    repoURL: oci://public.ecr.aws/aws-controllers-k8s/iam-chart
    targetRevision: v0.0.25
    helm:
      values: |
        serviceAccount:
          create: false
          name: ack-iam-controller
        aws:
          region: us-east-2
  destination:
    server: https://kubernetes.default.svc
    namespace: ack-system
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
```

Apply via ArgoCD:
```bash
kubectl apply -f argocd/apps/infrastructure/ack-iam-controller.yaml

# Or sync via ArgoCD CLI
argocd app sync ack-iam-controller
```

Verify installation:
```bash
kubectl get pods -n ack-system
kubectl logs -n ack-system -l app.kubernetes.io/name=iam-chart
```

### Step 3: Create IAM Roles for RDS and ElastiCache Controllers

Now that the ACK IAM controller is running, we can create IAM roles via Kubernetes CRDs!

**RDS Controller Role:**

```yaml
# k8s/ack/iam-roles/rds-controller-role.yaml
apiVersion: iam.services.k8s.aws/v1alpha1
kind: Role
metadata:
  name: ack-rds-controller-role
  namespace: ack-system
spec:
  name: ack-rds-controller-role
  description: IAM role for ACK RDS controller
  assumeRolePolicyDocument: |
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
              "OIDC_PROVIDER:sub": "system:serviceaccount:ack-system:ack-rds-controller",
              "OIDC_PROVIDER:aud": "sts.amazonaws.com"
            }
          }
        }
      ]
    }
  policies:
    - policyName: ACKRDSControllerPolicy
      policyDocument: |
        {
          "Version": "2012-10-17",
          "Statement": [
            {
              "Effect": "Allow",
              "Action": [
                "rds:CreateDBInstance",
                "rds:DeleteDBInstance",
                "rds:DescribeDBInstances",
                "rds:ModifyDBInstance",
                "rds:AddTagsToResource",
                "rds:RemoveTagsFromResource",
                "rds:ListTagsForResource",
                "rds:CreateDBSubnetGroup",
                "rds:DeleteDBSubnetGroup",
                "rds:DescribeDBSubnetGroups",
                "rds:ModifyDBSubnetGroup",
                "ec2:DescribeSecurityGroups",
                "ec2:DescribeSubnets",
                "ec2:DescribeVpcs"
              ],
              "Resource": "*"
            }
          ]
        }
```

**ElastiCache Controller Role:**

```yaml
# k8s/ack/iam-roles/elasticache-controller-role.yaml
apiVersion: iam.services.k8s.aws/v1alpha1
kind: Role
metadata:
  name: ack-elasticache-controller-role
  namespace: ack-system
spec:
  name: ack-elasticache-controller-role
  description: IAM role for ACK ElastiCache controller
  assumeRolePolicyDocument: |
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
              "OIDC_PROVIDER:sub": "system:serviceaccount:ack-system:ack-elasticache-controller",
              "OIDC_PROVIDER:aud": "sts.amazonaws.com"
            }
          }
        }
      ]
    }
  policies:
    - policyName: ACKElastiCacheControllerPolicy
      policyDocument: |
        {
          "Version": "2012-10-17",
          "Statement": [
            {
              "Effect": "Allow",
              "Action": [
                "elasticache:CreateCacheCluster",
                "elasticache:DeleteCacheCluster",
                "elasticache:DescribeCacheClusters",
                "elasticache:ModifyCacheCluster",
                "elasticache:AddTagsToResource",
                "elasticache:RemoveTagsFromResource",
                "elasticache:ListTagsForResource",
                "elasticache:CreateCacheSubnetGroup",
                "elasticache:DeleteCacheSubnetGroup",
                "elasticache:DescribeCacheSubnetGroups",
                "elasticache:ModifyCacheSubnetGroup",
                "ec2:DescribeSecurityGroups",
                "ec2:DescribeSubnets",
                "ec2:DescribeVpcs"
              ],
              "Resource": "*"
            }
          ]
        }
```

Apply the roles:
```bash
# Replace ACCOUNT_ID and OIDC_PROVIDER with your values
kubectl apply -f k8s/ack/iam-roles/

# Verify roles are created
kubectl get roles.iam.services.k8s.aws -n ack-system
kubectl describe role.iam.services.k8s.aws ack-rds-controller-role -n ack-system
```

### Step 4: Install RDS and ElastiCache Controllers

**RDS Controller:**

```yaml
# argocd/apps/infrastructure/ack-rds-controller.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: ack-rds-controller
  namespace: argocd
spec:
  project: default
  source:
    repoURL: oci://public.ecr.aws/aws-controllers-k8s/rds-chart
    targetRevision: v0.0.25
    helm:
      values: |
        serviceAccount:
          annotations:
            eks.amazonaws.com/role-arn: arn:aws:iam::ACCOUNT_ID:role/ack-rds-controller-role
        aws:
          region: us-east-2
  destination:
    server: https://kubernetes.default.svc
    namespace: ack-system
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
```

**ElastiCache Controller:**

```yaml
# argocd/apps/infrastructure/ack-elasticache-controller.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: ack-elasticache-controller
  namespace: argocd
spec:
  project: default
  source:
    repoURL: oci://public.ecr.aws/aws-controllers-k8s/elasticache-chart
    targetRevision: v0.0.25
    helm:
      values: |
        serviceAccount:
          annotations:
            eks.amazonaws.com/role-arn: arn:aws:iam::ACCOUNT_ID:role/ack-elasticache-controller-role
        aws:
          region: us-east-2
  destination:
    server: https://kubernetes.default.svc
    namespace: ack-system
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
```

Apply controllers:
```bash
kubectl apply -f argocd/apps/infrastructure/ack-rds-controller.yaml
kubectl apply -f argocd/apps/infrastructure/ack-elasticache-controller.yaml

# Verify controllers are running
kubectl get pods -n ack-system
kubectl logs -n ack-system -l app.kubernetes.io/name=rds-chart
kubectl logs -n ack-system -l app.kubernetes.io/name=elasticache-chart
```

## Creating AWS Resources

### RDS PostgreSQL Database

```yaml
# k8s/ack/databases/hydrocoptic-datastore-rds.yaml
apiVersion: rds.services.k8s.aws/v1alpha1
kind: DBInstance
metadata:
  name: hydrocoptic-datastore
  namespace: encabulator-data
spec:
  dbInstanceIdentifier: encabulator-hydrocoptic-db
  dbInstanceClass: db.t3.medium
  engine: postgres
  engineVersion: "15.4"
  masterUsername: encabulator
  masterUserPassword:
    namespace: encabulator-data
    name: hydrocoptic-db-credentials
    key: password
  allocatedStorage: 20
  storageType: gp3
  storageEncrypted: true
  publiclyAccessible: false
  vpcSecurityGroupIDs:
    - sg-XXXXXXXXX  # Replace with your security group
  dbSubnetGroupName: encabulator-db-subnet-group
  backupRetentionPeriod: 7
  preferredBackupWindow: "03:00-04:00"
  preferredMaintenanceWindow: "sun:04:00-sun:05:00"
  tags:
    - key: Environment
      value: production
    - key: ManagedBy
      value: ACK
    - key: Service
      value: hydrocoptic-datastore
```

### ElastiCache Redis Cluster

```yaml
# k8s/ack/caches/prefabulated-cache-redis.yaml
apiVersion: elasticache.services.k8s.aws/v1alpha1
kind: CacheCluster
metadata:
  name: prefabulated-cache
  namespace: encabulator-data
spec:
  cacheClusterID: encabulator-prefabulated-cache
  cacheNodeType: cache.t3.medium
  engine: redis
  engineVersion: "7.0"
  numCacheNodes: 1
  cacheSubnetGroupName: encabulator-cache-subnet-group
  securityGroupIDs:
    - sg-XXXXXXXXX  # Replace with your security group
  preferredMaintenanceWindow: "sun:05:00-sun:06:00"
  snapshotRetentionLimit: 5
  snapshotWindow: "03:00-05:00"
  tags:
    - key: Environment
      value: production
    - key: ManagedBy
      value: ACK
    - key: Service
      value: prefabulated-cache
```

Apply resources:
```bash
# Create secrets first
kubectl create secret generic hydrocoptic-db-credentials \
  -n encabulator-data \
  --from-literal=password='YOUR_SECURE_PASSWORD'

# Apply ACK resources
kubectl apply -f k8s/ack/databases/
kubectl apply -f k8s/ack/caches/

# Monitor creation (takes 10-15 minutes)
kubectl get dbinstances.rds.services.k8s.aws -n encabulator-data -w
kubectl get cacheclusters.elasticache.services.k8s.aws -n encabulator-data -w
```

## Updating Services to Use AWS Resources

Once AWS resources are ready, update service Helm values:

**Hydrocoptic DataStore:**

```yaml
# helm/charts/hydrocoptic-datastore/values.yaml
database:
  # Use RDS endpoint instead of in-cluster PostgreSQL
  host: encabulator-hydrocoptic-db.XXXXXXXXX.us-east-2.rds.amazonaws.com
  port: 5432
  name: postgres
  # Reference the same secret used by ACK
  existingSecret: hydrocoptic-db-credentials
  secretKeys:
    username: username
    password: password
```

**Prefabulated Cache:**

```yaml
# helm/charts/prefabulated-cache/values.yaml
redis:
  # Use ElastiCache endpoint instead of in-cluster Redis
  host: encabulator-prefabulated-cache.XXXXXX.0001.use2.cache.amazonaws.com
  port: 6379
```

Commit and push changes - ArgoCD will sync automatically!

## Verification

```bash
# Check ACK resources status
kubectl get dbinstances.rds.services.k8s.aws -n encabulator-data
kubectl get cacheclusters.elasticache.services.k8s.aws -n encabulator-data

# Check service connectivity
kubectl logs -n encabulator-data -l app=hydrocoptic-datastore
kubectl logs -n encabulator-data -l app=prefabulated-cache

# Test data flow
kubectl port-forward -n encabulator-control svc/cardinal-grammeter 8081:8080
curl -X POST http://localhost:8081/traffic/start \
  -H "Content-Type: application/json" \
  -d '{"target": "http://dingle-arm-api-gateway.encabulator-processing.svc.cluster.local:8080/api/v1/encabulations", "pattern": "steady", "fluorescent_score_motion": 100}'

# Verify data in RDS
kubectl port-forward -n encabulator-data svc/hydrocoptic-datastore 8082:8080
curl http://localhost:8082/api/v1/encabulations?pageSize=10
```

## Cleanup

To remove AWS resources:

```bash
# Delete ACK resources (this deletes AWS resources!)
kubectl delete -f k8s/ack/databases/
kubectl delete -f k8s/ack/caches/

# Uninstall controllers
kubectl delete -f argocd/apps/infrastructure/ack-rds-controller.yaml
kubectl delete -f argocd/apps/infrastructure/ack-elasticache-controller.yaml
kubectl delete -f argocd/apps/infrastructure/ack-iam-controller.yaml

# Delete IAM roles (manual)
aws iam delete-role --role-name ack-rds-controller-role
aws iam delete-role --role-name ack-elasticache-controller-role
aws iam delete-role --role-name ack-iam-controller-admin-role
```

## Troubleshooting

### Controller Not Starting

```bash
# Check controller logs
kubectl logs -n ack-system -l app.kubernetes.io/name=iam-chart
kubectl logs -n ack-system -l app.kubernetes.io/name=rds-chart

# Check service account annotations
kubectl get sa -n ack-system ack-iam-controller -o yaml
kubectl get sa -n ack-system ack-rds-controller -o yaml

# Verify IAM role trust policy
aws iam get-role --role-name ack-iam-controller-admin-role
```

### Resource Creation Failing

```bash
# Check ACK resource status
kubectl describe dbinstance.rds.services.k8s.aws hydrocoptic-datastore -n encabulator-data

# Check controller logs for errors
kubectl logs -n ack-system -l app.kubernetes.io/name=rds-chart --tail=100

# Verify IAM permissions
aws iam simulate-principal-policy \
  --policy-source-arn arn:aws:iam::ACCOUNT_ID:role/ack-rds-controller-role \
  --action-names rds:CreateDBInstance
```

### Service Cannot Connect to AWS Resource

```bash
# Check security groups
aws ec2 describe-security-groups --group-ids sg-XXXXXXXXX

# Check VPC and subnets
aws ec2 describe-subnets --subnet-ids subnet-XXXXXXXXX

# Test connectivity from pod
kubectl run -it --rm debug --image=postgres:15 --restart=Never -- \
  psql -h encabulator-hydrocoptic-db.XXXXXXXXX.us-east-2.rds.amazonaws.com -U encabulator
```

## Cost Optimization

- **Development**: Use db.t3.micro and cache.t3.micro (~$30/month)
- **Production**: Use db.t3.medium and cache.t3.medium (~$110/month)
- **Enable deletion protection** for production databases
- **Use Reserved Instances** for long-term deployments (up to 60% savings)

## Security Best Practices

1. **Use VPC-only resources** (no public access)
2. **Enable encryption at rest** for RDS and ElastiCache
3. **Use Secrets Manager** for database credentials (instead of Kubernetes Secrets)
4. **Enable audit logging** for RDS
5. **Use least-privilege IAM policies** for ACK controllers
6. **Enable automatic backups** with appropriate retention

## References

- [ACK Documentation](https://aws-controllers-k8s.github.io/community/)
- [ACK IAM Controller](https://github.com/aws-controllers-k8s/iam-controller)
- [ACK RDS Controller](https://github.com/aws-controllers-k8s/rds-controller)
- [ACK ElastiCache Controller](https://github.com/aws-controllers-k8s/elasticache-controller)
- [EKS Pod Identity](https://docs.aws.amazon.com/eks/latest/userguide/pod-identities.html)

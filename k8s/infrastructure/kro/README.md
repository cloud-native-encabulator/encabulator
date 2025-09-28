# Kube Resource Orchestrator (KRO) Implementation

This directory contains the Kube Resource Orchestrator (KRO) implementation for the Cloud Native Encabulator. KRO enables us to create custom Kubernetes APIs that compose multiple resources into single, manageable units.

## What is KRO and Why Use It?

Kube Resource Orchestrator (KRO) allows us to create **custom Kubernetes APIs** that compose multiple resources into single, manageable units. Instead of managing individual ACK resources (DBInstance, ReplicationGroup, IAM roles, security groups, etc.), we can create high-level APIs that encapsulate entire infrastructure stacks.

## KRO Compositions

### 1. `EncabulatorDatabase` - Complete Database Stack

**Simple Usage:**
```yaml
apiVersion: encabulator.io/v1alpha1
kind: EncabulatorDatabase
metadata:
  name: hydrocoptic-datastore
spec:
  environment: development
  instanceClass: db.t3.micro
  storage: 20
  multiAZ: false
```

**What it creates behind the scenes:**
- ACK `DBInstance` (PostgreSQL)
- ACK `DBSubnetGroup` 
- Security groups for database access
- IAM roles for database access
- Kubernetes `Secret` with connection details
- `ServiceMonitor` for Prometheus metrics

### 2. `EncabulatorCache` - Complete Cache Stack

**Simple Usage:**
```yaml
apiVersion: encabulator.io/v1alpha1
kind: EncabulatorCache
metadata:
  name: prefabulated-cache
spec:
  environment: development
  nodeType: cache.t3.micro
  replicas: 1
  encryption: true
```

**What it creates behind the scenes:**
- ACK `ReplicationGroup` (Redis)
- ACK `CacheSubnetGroup`
- Security groups for cache access
- IAM roles for cache access
- Kubernetes `Secret` with connection details
- `ServiceMonitor` for Prometheus metrics

### 3. `EncabulatorCluster` - Complete Environment Stack

**Simple Usage:**
```yaml
apiVersion: encabulator.io/v1alpha1
kind: EncabulatorCluster
metadata:
  name: development-cluster
spec:
  environment: development
  database:
    instanceClass: db.t3.micro
    storage: 20
  cache:
    nodeType: cache.t3.micro
    replicas: 1
  monitoring: true
  networking:
    ingressEnabled: true
```

**What it creates behind the scenes:**
- `EncabulatorDatabase` (which creates all DB resources)
- `EncabulatorCache` (which creates all cache resources)
- Networking resources (VPC, subnets, security groups)
- Monitoring resources (ServiceMonitors, dashboards)
- Ingress configuration

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    KRO Custom APIs                         │
├─────────────────────────────────────────────────────────────┤
│  EncabulatorCluster  │  EncabulatorDatabase │ EncabulatorCache │
│         │            │          │           │        │        │
│         ▼            │          ▼           │        ▼        │
├─────────────────────────────────────────────────────────────┤
│                   KRO Controllers                          │
├─────────────────────────────────────────────────────────────┤
│         │            │          │           │        │        │
│         ▼            │          ▼           │        ▼        │
├─────────────────────────────────────────────────────────────┤
│    ACK Resources (DBInstance, ReplicationGroup, IAM, etc.)  │
├─────────────────────────────────────────────────────────────┤
│         │            │          │           │        │        │
│         ▼            │          ▼           │        ▼        │
├─────────────────────────────────────────────────────────────┤
│              AWS Resources (RDS, ElastiCache, IAM)          │
└─────────────────────────────────────────────────────────────┘
```

## Benefits

### 1. **Simplified Operations**
Instead of this complex deployment:
```bash
kubectl apply -f rds-instance.yaml
kubectl apply -f security-groups.yaml
kubectl apply -f iam-roles.yaml
kubectl apply -f secrets.yaml
kubectl apply -f elasticache.yaml
kubectl apply -f monitoring.yaml
```

We get this simple deployment:
```bash
kubectl apply -f encabulator-cluster.yaml
```

### 2. **Platform Engineering Approach**
- **Platform team** creates and maintains KRO compositions
- **Development teams** use simple, high-level APIs
- **Operations** benefit from consistent, standardized deployments

### 3. **Key Advantages**
- **Consistency**: Standardized patterns across environments
- **Encapsulation**: Hide complexity behind clean APIs
- **Reusability**: Same compositions work across dev/staging/prod
- **GitOps Friendly**: High-level resources are easier to manage in Git
- **Reduced Errors**: Fewer individual resources to manage

## Directory Structure

```
k8s/infrastructure/kro/
├── README.md                    # This documentation
├── kro-installation/           # KRO controller installation
│   ├── kustomization.yaml
│   ├── namespace.yaml
│   ├── kro-controller.yaml
│   └── rbac.yaml
├── resource-groups/            # KRO ResourceGroup definitions
│   ├── encabulator-database.yaml
│   ├── encabulator-cache.yaml
│   └── encabulator-cluster.yaml
├── templates/                  # Resource templates for compositions
│   ├── database/
│   ├── cache/
│   └── cluster/
└── examples/                   # Example usage of KRO compositions
    ├── development-cluster.yaml
    ├── staging-cluster.yaml
    └── production-cluster.yaml
```

## Installation

### Prerequisites
- Kubernetes cluster with ACK controllers installed
- ArgoCD (for GitOps deployment)
- Sufficient RBAC permissions

### Via ArgoCD (Recommended)
```bash
# KRO is deployed automatically via infrastructure-root application
./scripts/platform/bootstrap-gitops.sh
```

### Manual Installation
```bash
# Install KRO controller
kubectl apply -k k8s/infrastructure/kro/kro-installation/

# Deploy ResourceGroup definitions
kubectl apply -k k8s/infrastructure/kro/resource-groups/

# Verify installation
kubectl get pods -n kro-system
kubectl get resourcegroups
```

## Usage Examples

### Development Environment
```yaml
apiVersion: encabulator.io/v1alpha1
kind: EncabulatorCluster
metadata:
  name: dev-cluster
spec:
  environment: development
  database:
    instanceClass: db.t3.micro
    storage: 20
    multiAZ: false
  cache:
    nodeType: cache.t3.micro
    replicas: 1
  monitoring: true
```

### Production Environment
```yaml
apiVersion: encabulator.io/v1alpha1
kind: EncabulatorCluster
metadata:
  name: prod-cluster
spec:
  environment: production
  database:
    instanceClass: db.r5.large
    storage: 100
    multiAZ: true
    backupRetention: 30
  cache:
    nodeType: cache.r5.large
    replicas: 3
    multiAZ: true
  monitoring: true
  alerting: true
  networking:
    ingressEnabled: true
    tlsEnabled: true
```

### Individual Components
```yaml
# Just a database
apiVersion: encabulator.io/v1alpha1
kind: EncabulatorDatabase
metadata:
  name: analytics-db
spec:
  environment: staging
  instanceClass: db.t3.medium
  storage: 50

---
# Just a cache
apiVersion: encabulator.io/v1alpha1
kind: EncabulatorCache
metadata:
  name: session-cache
spec:
  environment: staging
  nodeType: cache.t3.small
  replicas: 2
```

## Monitoring and Observability

### Check KRO Status
```bash
# Check KRO controller
kubectl get pods -n kro-system

# List available ResourceGroups
kubectl get resourcegroups

# Check custom resources
kubectl get encabulatordatabase
kubectl get encabulatorcache
kubectl get encabulatorcluster
```

### Monitor Resource Creation
```bash
# Watch resources being created
kubectl get all --all-namespaces -w

# Check specific composition status
kubectl describe encabulatorcluster dev-cluster

# View underlying resources
kubectl get dbinstance,replicationgroup,role --all-namespaces
```

## Troubleshooting

### Common Issues

1. **KRO Controller not starting**:
   ```bash
   kubectl logs -n kro-system deployment/kro-controller
   kubectl describe pod -n kro-system
   ```

2. **ResourceGroup not creating CRDs**:
   ```bash
   kubectl describe resourcegroup encabulator-database
   kubectl get events --all-namespaces
   ```

3. **Custom resources not creating underlying resources**:
   ```bash
   kubectl describe encabulatorcluster <name>
   kubectl get events -n <namespace>
   ```

### Debugging Commands
```bash
# Check KRO controller logs
kubectl logs -n kro-system deployment/kro-controller

# Check ResourceGroup status
kubectl get resourcegroups -o wide

# Check custom resource status
kubectl get encabulatorcluster <name> -o yaml

# Check underlying ACK resources
kubectl get dbinstance,replicationgroup --all-namespaces
```

## Customization

### Adding New Compositions

1. **Create ResourceGroup definition**:
   ```yaml
   apiVersion: kro.run/v1alpha1
   kind: ResourceGroup
   metadata:
     name: encabulator-monitoring
   spec:
     schema:
       # Define API schema
     resources:
       # Define resource templates
   ```

2. **Test the composition**:
   ```bash
   kubectl apply -f new-resource-group.yaml
   kubectl get crd | grep encabulator
   ```

3. **Create example usage**:
   ```yaml
   apiVersion: encabulator.io/v1alpha1
   kind: EncabulatorMonitoring
   metadata:
     name: dev-monitoring
   spec:
     # Use your new API
   ```

### Environment-Specific Overlays

Use Kustomize overlays for environment-specific configurations:

```
overlays/
├── development/
│   ├── kustomization.yaml
│   └── dev-cluster.yaml
├── staging/
│   ├── kustomization.yaml
│   └── staging-cluster.yaml
└── production/
    ├── kustomization.yaml
    └── prod-cluster.yaml
```

## Security Considerations

### RBAC
- KRO controller has minimal required permissions
- Custom resource access controlled via Kubernetes RBAC
- Separate permissions for different environments

### Resource Isolation
- Compositions create resources in appropriate namespaces
- Network policies restrict inter-resource communication
- IAM roles follow principle of least privilege

### Secrets Management
- Database and cache credentials stored as Kubernetes Secrets
- Integration with external secret management systems
- Automatic secret rotation capabilities

## Integration with Cloud Native Encabulator

### GitOps Workflow
1. **Platform team** maintains KRO compositions in Git
2. **ArgoCD** deploys ResourceGroups and creates custom APIs
3. **Development teams** use high-level APIs in their applications
4. **Operations** monitor and manage through standard Kubernetes tools

### Service Integration
- **Data Plane services** automatically discover database and cache endpoints
- **Monitoring stack** automatically scrapes metrics from created resources
- **Control Plane** provides UI for managing KRO compositions

### Scaling and Performance
- Compositions support horizontal scaling parameters
- Resource requests and limits defined in templates
- Performance monitoring integrated with observability stack

This KRO implementation transforms infrastructure management from complex, error-prone manual processes into simple, declarative, platform-engineering approaches that scale across teams and environments.
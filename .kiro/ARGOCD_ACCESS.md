# ArgoCD Access Information

**Installation Date:** 2025-11-02  
**Namespace:** `argocd`

## Access Credentials

**Username:** `admin`  
**Initial Password:** `rXd2XFVb3M7cp6hh`

> **Note:** Change this password after first login for security.

## Accessing ArgoCD UI

### Via Port Forward

```bash
# Start port-forward (run in background)
kubectl port-forward svc/argocd-server -n argocd 8080:443

# Access UI at:
# https://localhost:8080
```

### Login

1. Navigate to https://localhost:8080
2. Accept the self-signed certificate warning
3. Login with username `admin` and password above

## ArgoCD CLI

### Installation

ArgoCD CLI is already installed via Homebrew:

```bash
which argocd
# /opt/homebrew/bin/argocd

argocd version --client
# argocd: v3.1.1+fa342d1
```

### CLI Login

```bash
# Login to ArgoCD server
argocd login localhost:8080 --username admin --password rXd2XFVb3M7cp6hh --insecure

# List clusters
argocd cluster list

# List applications
argocd app list
```

## Common Commands

```bash
# Get ArgoCD pods status
kubectl get pods -n argocd

# Get ArgoCD services
kubectl get svc -n argocd

# View ArgoCD server logs
kubectl logs -n argocd deployment/argocd-server

# View application controller logs
kubectl logs -n argocd statefulset/argocd-application-controller
```

## Change Admin Password

```bash
# Via CLI (recommended)
argocd account update-password

# Or via UI
# Settings → Accounts → admin → Update Password
```

## Cluster Information

**Cluster:** `butlerjl@molly.us-east-2.eksctl.io`  
**Context:** `butlerjl@molly.us-east-2.eksctl.io`  
**Region:** `us-east-2`

## ArgoCD Components

All components running in `argocd` namespace:

- **argocd-server** - API server and UI
- **argocd-repo-server** - Repository server (Git operations)
- **argocd-application-controller** - Application controller (sync operations)
- **argocd-dex-server** - SSO/OIDC integration
- **argocd-redis** - Cache and state storage
- **argocd-applicationset-controller** - ApplicationSet controller
- **argocd-notifications-controller** - Notifications

## Next Steps

1. ✅ ArgoCD installed and running
2. ✅ CLI configured and authenticated
3. ✅ UI accessible via port-forward
4. ⏭️ Create ArgoCD Application manifests
5. ⏭️ Deploy applications via GitOps

## Troubleshooting

### Port-forward not working

```bash
# Kill existing port-forward
pkill -f "port-forward.*argocd-server"

# Start new port-forward
kubectl port-forward svc/argocd-server -n argocd 8080:443
```

### Can't access UI

```bash
# Check ArgoCD server is running
kubectl get pods -n argocd | grep argocd-server

# Check service
kubectl get svc -n argocd argocd-server

# Check logs
kubectl logs -n argocd deployment/argocd-server
```

### CLI login fails

```bash
# Verify port-forward is running
curl -k https://localhost:8080/healthz

# Re-login
argocd login localhost:8080 --username admin --password rXd2XFVb3M7cp6hh --insecure
```

## Security Notes

- Initial admin password is stored in `argocd-initial-admin-secret` secret
- Change password after first login
- Consider setting up SSO/OIDC for production use
- Use RBAC to control access to applications and clusters
- Enable audit logging for compliance

## Documentation

- Official Docs: https://argo-cd.readthedocs.io/
- Getting Started: https://argo-cd.readthedocs.io/en/stable/getting_started/
- User Guide: https://argo-cd.readthedocs.io/en/stable/user-guide/
- Operator Manual: https://argo-cd.readthedocs.io/en/stable/operator-manual/

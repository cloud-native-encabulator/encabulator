# Inputs. Names the other parts of the talk depend on (cluster, stores, namespace) are
# locals in main.tf, not variables, so they cannot drift from the shared contract.
# Real values go in a gitignored *.auto.tfvars; see terraform.tfvars.example.

variable "region" {
  description = "Region for the hub (Act 1 starting point)."
  type        = string
  default     = "us-west-2"
}

variable "kubernetes_version" {
  description = "EKS Kubernetes minor version for the hub cluster."
  type        = string
  default     = "1.37"
}

variable "vpc_cidr" {
  description = "CIDR for the hub VPC. Split into three private and three public subnets."
  type        = string
  default     = "10.40.0.0/16"
}

variable "postgres_engine_version" {
  description = "RDS for PostgreSQL major version. 18 is the newest major RDS offers outside the preview environment (19 is beta only, October 2026)."
  type        = string
  default     = "18"
}

variable "db_instance_class" {
  description = "RDS instance class for encabulator-pg."
  type        = string
  default     = "db.t4g.medium"
}

variable "valkey_major_engine_version" {
  description = "ElastiCache serverless Valkey major version. 8 this round (contract D2); ElastiCache lists Valkey 9.0 and 9.1, verify 9 on Serverless before moving. Must match the ACK ServerlessCache majorEngineVersion."
  type        = string
  default     = "8"
}

# ---------------------------------------------------------------- Argo CD capability

variable "idc_instance_arn" {
  description = "ARN of the IAM Identity Center instance the Argo CD capability signs users in with (arn:aws:sso:::instance/ssoins-...)."
  type        = string

  validation {
    condition     = can(regex("^arn:aws[a-z-]*:sso:::instance/ssoins-[0-9a-f]+$", var.idc_instance_arn))
    error_message = "idc_instance_arn must look like arn:aws:sso:::instance/ssoins-0123456789abcdef."
  }
}

variable "idc_region" {
  description = "Region of the IAM Identity Center instance. null means the hub region."
  type        = string
  default     = null
}

variable "argocd_rbac_role_mappings" {
  description = "Identity Center users or groups mapped to Argo CD roles (ADMIN, EDITOR, VIEWER). Without one (or argocd_admin_group_id) nobody can sign in to the Argo CD UI; kubectl still works. When set, this list wins over argocd_admin_group_id."
  type = list(object({
    role = string
    identities = list(object({
      id   = string
      type = string # SSO_USER or SSO_GROUP
    }))
  }))
  default = []

  validation {
    condition     = alltrue([for m in var.argocd_rbac_role_mappings : contains(["ADMIN", "EDITOR", "VIEWER"], m.role)])
    error_message = "role must be ADMIN, EDITOR or VIEWER."
  }
}

variable "argocd_admin_group_id" {
  description = "Id of one Identity Center group whose members become Argo CD admins: the short form of an argocd_rbac_role_mappings list with one ADMIN SSO_GROUP. Used only when argocd_rbac_role_mappings is empty; empty means no such mapping. `make hub` sets it from ARGOCD_ADMIN_GROUP_ID in .env."
  type        = string
  default     = ""

  validation {
    condition     = var.argocd_admin_group_id == "" || can(regex("^([0-9a-f]{10}-)?[A-Fa-f0-9]{8}-[A-Fa-f0-9]{4}-[A-Fa-f0-9]{4}-[A-Fa-f0-9]{4}-[A-Fa-f0-9]{12}$", var.argocd_admin_group_id))
    error_message = "argocd_admin_group_id must be an Identity Center group id (aws identitystore list-groups), for example 00000000-0000-0000-0000-000000000000."
  }
}

# ------------------------------------------------- capability Kubernetes permissions

variable "argocd_cluster_admin" {
  description = "DEMO ONLY. Associate AmazonEKSClusterAdminPolicy with the Argo CD capability on the hub so it can sync RGDs, RBAC and ACK resources anywhere. Scope it down for real use."
  type        = bool
  default     = true
}

variable "kro_cluster_admin" {
  description = "DEMO ONLY fallback. Associate AmazonEKSClusterAdminPolicy with the kro capability on the hub. Off by default: kro's hub permissions come from the group encabulator:kro (on its access entry), bound by gitops/hub/rbac."
  type        = bool
  default     = false
}

variable "ack_regions" {
  description = "Regions the ACK capability may manage resources in: the hub plus every cell region."
  type        = list(string)
  default     = ["us-west-2", "us-east-2", "eu-west-1"]
}

variable "ack_iam_role_prefixes" {
  description = "IAM role name prefixes the ACK capability may create, change and pass (the cells' roles). The hub's own roles (encabulator-usw2-*) are deliberately not in this list."
  type        = list(string)
  default     = ["encabulator-use2-", "encabulator-reh1-", "encabulator-euw1-"]
}

variable "ack_eks_cluster_names" {
  description = "EKS clusters (cells) the ACK capability may create and manage, with their capabilities and access entries. The hub cluster is deliberately not in this list."
  type        = list(string)
  default     = ["encabulator-use2", "encabulator-reh1", "encabulator-euw1"]
}

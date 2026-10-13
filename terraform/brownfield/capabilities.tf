# EKS Capabilities on the hub: Argo CD, ACK and kro, each with its own capability role.
# The controllers run in AWS-managed infrastructure, not on the cluster's nodes.

locals {
  capability_roles = {
    argocd = "${local.cluster_name}-argocd"
    ack    = "${local.cluster_name}-ack"
    kro    = "${local.cluster_name}-kro"
  }

  # Kubernetes group on the kro capability's access entry (contract D4). gitops/hub/rbac
  # and gitops/app/rbac bind RBAC to this group, never to the role ARN.
  kro_kubernetes_group = "encabulator:kro"

  access_policy_prefix = "arn:${data.aws_partition.current.partition}:eks::aws:cluster-access-policy"
  account_id           = data.aws_caller_identity.current.account_id
  partition            = data.aws_partition.current.partition
}

data "aws_iam_policy_document" "capability_assume" {
  statement {
    actions = ["sts:AssumeRole", "sts:TagSession"]
    principals {
      type        = "Service"
      identifiers = ["capabilities.eks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "capability" {
  for_each = local.capability_roles

  name               = each.value
  description        = "EKS ${each.key} capability role for ${local.cluster_name}"
  assume_role_policy = data.aws_iam_policy_document.capability_assume.json
}

# Argo CD reads a public GitHub repository and kro composes Kubernetes objects: neither
# role needs AWS permissions beyond the trust policy.

# ------------------------------------------------------------------- ACK permissions
# Simple setup (all permissions on the capability role), scoped to what the talk needs:
#   Act 1  adopt RDS, ElastiCache and the DB subnet group in the hub region
#   Act 3  build a cell in us-east-2: VPC pieces, IAM roles for the cell, the EKS cluster,
#          its capabilities and access entries
# For production use IAM role selectors instead (one role per namespace or team).

data "aws_iam_policy_document" "ack" {
  statement {
    sid       = "Rds"
    actions   = ["rds:*"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "aws:RequestedRegion"
      values   = var.ack_regions
    }
  }

  statement {
    sid       = "ElastiCache"
    actions   = ["elasticache:*"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "aws:RequestedRegion"
      values   = var.ack_regions
    }
  }

  # Encrypted RDS storage with the AWS managed key.
  statement {
    sid       = "KmsViaRds"
    actions   = ["kms:DescribeKey", "kms:CreateGrant"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "kms:ViaService"
      values   = [for r in var.ack_regions : "rds.${r}.amazonaws.com"]
    }
  }

  statement {
    sid = "Ec2Networking"
    actions = [
      "ec2:Describe*",
      "ec2:CreateTags",
      "ec2:DeleteTags",
      # VPC
      "ec2:CreateVpc",
      "ec2:DeleteVpc",
      "ec2:ModifyVpcAttribute",
      "ec2:AssociateVpcCidrBlock",
      "ec2:DisassociateVpcCidrBlock",
      # Subnets
      "ec2:CreateSubnet",
      "ec2:DeleteSubnet",
      "ec2:ModifySubnetAttribute",
      # Security groups
      "ec2:CreateSecurityGroup",
      "ec2:DeleteSecurityGroup",
      "ec2:AuthorizeSecurityGroupIngress",
      "ec2:AuthorizeSecurityGroupEgress",
      "ec2:RevokeSecurityGroupIngress",
      "ec2:RevokeSecurityGroupEgress",
      "ec2:ModifySecurityGroupRules",
      "ec2:UpdateSecurityGroupRuleDescriptionsIngress",
      "ec2:UpdateSecurityGroupRuleDescriptionsEgress",
      # Route tables
      "ec2:CreateRouteTable",
      "ec2:DeleteRouteTable",
      "ec2:CreateRoute",
      "ec2:DeleteRoute",
      "ec2:ReplaceRoute",
      "ec2:AssociateRouteTable",
      "ec2:DisassociateRouteTable",
      "ec2:ReplaceRouteTableAssociation",
      # Internet gateway
      "ec2:CreateInternetGateway",
      "ec2:DeleteInternetGateway",
      "ec2:AttachInternetGateway",
      "ec2:DetachInternetGateway",
      # NAT gateway and its Elastic IP
      "ec2:CreateNatGateway",
      "ec2:DeleteNatGateway",
      "ec2:AllocateAddress",
      "ec2:ReleaseAddress",
      "ec2:AssociateAddress",
      "ec2:DisassociateAddress",
    ]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "aws:RequestedRegion"
      values   = var.ack_regions
    }
  }

  # Cell clusters only: the hub cluster is not in this list, so ACK cannot change it.
  statement {
    sid     = "EksCells"
    actions = ["eks:*"]
    resources = flatten([
      for name in var.ack_eks_cluster_names : [
        "arn:${local.partition}:eks:*:${local.account_id}:cluster/${name}",
        "arn:${local.partition}:eks:*:${local.account_id}:access-entry/${name}/*",
        "arn:${local.partition}:eks:*:${local.account_id}:capability/${name}/*",
        "arn:${local.partition}:eks:*:${local.account_id}:addon/${name}/*",
        "arn:${local.partition}:eks:*:${local.account_id}:podidentityassociation/${name}/*",
      ]
    ])
    condition {
      test     = "StringEquals"
      variable = "aws:RequestedRegion"
      values   = var.ack_regions
    }
  }

  statement {
    sid = "EksRead"
    actions = [
      "eks:ListClusters",
      "eks:ListAccessPolicies",
      "eks:DescribeClusterVersions",
      "eks:DescribeAddonVersions",
    ]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "aws:RequestedRegion"
      values   = var.ack_regions
    }
  }

  # IAM roles for the cells (encabulator-use2-*). The hub's own roles are not covered,
  # so ACK cannot widen its own permissions.
  statement {
    sid = "IamCellRoles"
    actions = [
      "iam:CreateRole",
      "iam:DeleteRole",
      "iam:GetRole",
      "iam:UpdateRole",
      "iam:UpdateRoleDescription",
      "iam:UpdateAssumeRolePolicy",
      "iam:TagRole",
      "iam:UntagRole",
      "iam:ListRoleTags",
      "iam:AttachRolePolicy",
      "iam:DetachRolePolicy",
      "iam:ListAttachedRolePolicies",
      "iam:PutRolePolicy",
      "iam:GetRolePolicy",
      "iam:DeleteRolePolicy",
      "iam:ListRolePolicies",
      "iam:PutRolePermissionsBoundary",
      "iam:DeleteRolePermissionsBoundary",
      "iam:ListInstanceProfilesForRole",
    ]
    resources = [for p in var.ack_iam_role_prefixes : "arn:${local.partition}:iam::${local.account_id}:role/${p}*"]
  }

  statement {
    sid       = "IamPassCellRoles"
    actions   = ["iam:PassRole"]
    resources = [for p in var.ack_iam_role_prefixes : "arn:${local.partition}:iam::${local.account_id}:role/${p}*"]
    condition {
      test     = "StringEquals"
      variable = "iam:PassedToService"
      values = [
        "eks.amazonaws.com",
        "capabilities.eks.amazonaws.com",
        "ec2.amazonaws.com",
      ]
    }
  }

  statement {
    sid       = "ServiceLinkedRoles"
    actions   = ["iam:CreateServiceLinkedRole"]
    resources = ["arn:${local.partition}:iam::${local.account_id}:role/aws-service-role/*"]
    condition {
      test     = "StringEquals"
      variable = "iam:AWSServiceName"
      values = [
        "eks.amazonaws.com",
        "rds.amazonaws.com",
        "elasticache.amazonaws.com",
      ]
    }
  }
}

resource "aws_iam_role_policy" "ack" {
  name   = "ack-encabulator"
  role   = aws_iam_role.capability["ack"].id
  policy = data.aws_iam_policy_document.ack.json
}

# ----------------------------------------------------------------------- capabilities

resource "aws_eks_capability" "argocd" {
  cluster_name              = aws_eks_cluster.hub.name
  capability_name           = "argocd"
  type                      = "ARGOCD"
  role_arn                  = aws_iam_role.capability["argocd"].arn
  delete_propagation_policy = "RETAIN"

  depends_on = [aws_eks_access_entry.capability]

  configuration {
    argo_cd {
      namespace = "argocd"

      aws_idc {
        idc_instance_arn = var.idc_instance_arn
        idc_region       = var.idc_region
      }

      dynamic "rbac_role_mapping" {
        for_each = var.argocd_rbac_role_mappings
        content {
          role = rbac_role_mapping.value.role

          dynamic "identity" {
            for_each = rbac_role_mapping.value.identities
            content {
              id   = identity.value.id
              type = identity.value.type
            }
          }
        }
      }
    }
  }
}

resource "aws_eks_capability" "ack" {
  cluster_name              = aws_eks_cluster.hub.name
  capability_name           = "ack"
  type                      = "ACK"
  role_arn                  = aws_iam_role.capability["ack"].arn
  delete_propagation_policy = "RETAIN"

  depends_on = [aws_iam_role_policy.ack, aws_eks_access_entry.capability]
}

# Every capability's access entry is created here, before the capability, so it carries the
# contract tags (an entry EKS creates for a capability carries none) and, for kro, the
# Kubernetes group that the GitOps RBAC binds (contract D4). EKS adds each capability's
# default access policies (Argo CD: AmazonEKSArgoCDClusterPolicy + AmazonEKSArgoCDPolicy in
# argocd; ACK: AmazonEKSACKPolicy; kro: AmazonEKSKROPolicy) to the existing entry when the
# capability is created. The cell (gitops/hub/rgds) pre-creates its entries the same way.
resource "aws_eks_access_entry" "capability" {
  for_each = toset(["argocd", "ack"])

  cluster_name  = aws_eks_cluster.hub.name
  principal_arn = aws_iam_role.capability[each.key].arn
  type          = "STANDARD"
}

resource "aws_eks_access_entry" "kro" {
  cluster_name      = aws_eks_cluster.hub.name
  principal_arn     = aws_iam_role.capability["kro"].arn
  type              = "STANDARD"
  kubernetes_groups = [local.kro_kubernetes_group]
}

resource "aws_eks_capability" "kro" {
  cluster_name              = aws_eks_cluster.hub.name
  capability_name           = "kro"
  type                      = "KRO"
  role_arn                  = aws_iam_role.capability["kro"].arn
  delete_propagation_policy = "RETAIN"

  depends_on = [aws_eks_access_entry.kro]
}

# ------------------------------------------------ extra Kubernetes permissions (hub)
# These associations add to the capability access entries above, after each capability has
# added its default policies.

# The rds controller reads the master password from the Secret encabulator-db.
resource "aws_eks_access_policy_association" "ack_secret_reader" {
  cluster_name  = aws_eks_cluster.hub.name
  principal_arn = aws_iam_role.capability["ack"].arn
  policy_arn    = "${local.access_policy_prefix}/AmazonEKSSecretReaderPolicy"

  access_scope {
    type       = "namespace"
    namespaces = [local.namespace]
  }

  depends_on = [aws_eks_capability.ack]
}

# DEMO ONLY: Argo CD syncs RGDs, RBAC, ACK resources and Secrets across namespaces on the
# hub. Cluster admin keeps the demo simple; scope it (namespaced access policies or RBAC
# on a group) for real use.
resource "aws_eks_access_policy_association" "argocd_cluster_admin" {
  count = var.argocd_cluster_admin ? 1 : 0

  cluster_name  = aws_eks_cluster.hub.name
  principal_arn = aws_iam_role.capability["argocd"].arn
  policy_arn    = "${local.access_policy_prefix}/AmazonEKSClusterAdminPolicy"

  access_scope {
    type = "cluster"
  }

  depends_on = [aws_eks_capability.argocd]
}

# DEMO ONLY fallback, off by default (see var.kro_cluster_admin).
resource "aws_eks_access_policy_association" "kro_cluster_admin" {
  count = var.kro_cluster_admin ? 1 : 0

  cluster_name  = aws_eks_cluster.hub.name
  principal_arn = aws_iam_role.capability["kro"].arn
  policy_arn    = "${local.access_policy_prefix}/AmazonEKSClusterAdminPolicy"

  access_scope {
    type = "cluster"
  }

  depends_on = [aws_eks_capability.kro]
}

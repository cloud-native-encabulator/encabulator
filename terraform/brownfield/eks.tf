# Hub cluster: EKS Auto Mode, API authentication, the creator as cluster admin.

# ------------------------------------------------------------------ cluster IAM role

data "aws_iam_policy_document" "cluster_assume" {
  statement {
    actions = ["sts:AssumeRole", "sts:TagSession"]
    principals {
      type        = "Service"
      identifiers = ["eks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "cluster" {
  name               = "${local.cluster_name}-cluster"
  description        = "EKS Auto Mode cluster role for ${local.cluster_name}"
  assume_role_policy = data.aws_iam_policy_document.cluster_assume.json
}

resource "aws_iam_role_policy_attachment" "cluster" {
  for_each = toset([
    "AmazonEKSClusterPolicy",
    "AmazonEKSComputePolicy",
    "AmazonEKSBlockStoragePolicyV2",
    "AmazonEKSLoadBalancingPolicy",
    "AmazonEKSNetworkingPolicy",
  ])

  role       = aws_iam_role.cluster.name
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/${each.value}"
}

# The Auto Mode managed policies only allow eks:, kubernetes.io/ and karpenter.sh/ tag
# keys, so a NodeClass or StorageClass that asks for auto-delete=no would be refused.
# This is the custom-tags policy from the EKS Auto Mode cluster IAM role guide.
data "aws_iam_policy_document" "cluster_custom_tags" {
  statement {
    sid       = "Compute"
    actions   = ["ec2:CreateFleet", "ec2:RunInstances", "ec2:CreateLaunchTemplate"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "aws:RequestTag/eks:eks-cluster-name"
      values   = ["$${aws:PrincipalTag/eks:eks-cluster-name}"]
    }
    condition {
      test     = "StringLike"
      variable = "aws:RequestTag/eks:kubernetes-node-class-name"
      values   = ["*"]
    }
    condition {
      test     = "StringLike"
      variable = "aws:RequestTag/eks:kubernetes-node-pool-name"
      values   = ["*"]
    }
  }

  statement {
    sid     = "Storage"
    actions = ["ec2:CreateVolume", "ec2:CreateSnapshot"]
    resources = [
      "arn:${data.aws_partition.current.partition}:ec2:*:*:volume/*",
      "arn:${data.aws_partition.current.partition}:ec2:*:*:snapshot/*",
    ]
    condition {
      test     = "StringEquals"
      variable = "aws:RequestTag/eks:eks-cluster-name"
      values   = ["$${aws:PrincipalTag/eks:eks-cluster-name}"]
    }
  }

  statement {
    sid       = "Networking"
    actions   = ["ec2:CreateNetworkInterface"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "aws:RequestTag/eks:eks-cluster-name"
      values   = ["$${aws:PrincipalTag/eks:eks-cluster-name}"]
    }
    condition {
      test     = "StringLike"
      variable = "aws:RequestTag/eks:kubernetes-cni-node-name"
      values   = ["*"]
    }
  }

  statement {
    sid = "LoadBalancer"
    actions = [
      "elasticloadbalancing:CreateLoadBalancer",
      "elasticloadbalancing:CreateTargetGroup",
      "elasticloadbalancing:CreateListener",
      "elasticloadbalancing:CreateRule",
      "ec2:CreateSecurityGroup",
    ]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "aws:RequestTag/eks:eks-cluster-name"
      values   = ["$${aws:PrincipalTag/eks:eks-cluster-name}"]
    }
  }

  statement {
    sid       = "ShieldProtection"
    actions   = ["shield:CreateProtection"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "aws:RequestTag/eks:eks-cluster-name"
      values   = ["$${aws:PrincipalTag/eks:eks-cluster-name}"]
    }
  }

  statement {
    sid       = "ShieldTagResource"
    actions   = ["shield:TagResource"]
    resources = ["arn:${data.aws_partition.current.partition}:shield::*:protection/*"]
    condition {
      test     = "StringEquals"
      variable = "aws:RequestTag/eks:eks-cluster-name"
      values   = ["$${aws:PrincipalTag/eks:eks-cluster-name}"]
    }
  }
}

resource "aws_iam_role_policy" "cluster_custom_tags" {
  name   = "auto-mode-custom-tags"
  role   = aws_iam_role.cluster.id
  policy = data.aws_iam_policy_document.cluster_custom_tags.json
}

# --------------------------------------------------------------------- node IAM role

data "aws_iam_policy_document" "node_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "node" {
  name               = "${local.cluster_name}-node"
  description        = "EKS Auto Mode node role for ${local.cluster_name}"
  assume_role_policy = data.aws_iam_policy_document.node_assume.json
}

resource "aws_iam_role_policy_attachment" "node" {
  for_each = toset([
    "AmazonEKSWorkerNodeMinimalPolicy",
    "AmazonEC2ContainerRegistryPullOnly",
  ])

  role       = aws_iam_role.node.name
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/${each.value}"
}

# ----------------------------------------------------------------------------- cluster

resource "aws_eks_cluster" "hub" {
  name     = local.cluster_name
  version  = var.kubernetes_version
  role_arn = aws_iam_role.cluster.arn

  # Auto Mode runs the networking, storage and load-balancing components itself.
  bootstrap_self_managed_addons = false

  access_config {
    authentication_mode = "API"
    # No EKS-created creator entry: it would carry no tags. The principal running
    # `terraform apply` gets an explicit, tagged access entry with
    # AmazonEKSClusterAdminPolicy instead (aws_eks_access_entry.admin below), which the
    # Kubernetes and Helm providers rely on.
    bootstrap_cluster_creator_admin_permissions = false
  }

  # Auto Mode on, built-in node pools off: the built-in pools use the managed "default"
  # NodeClass, which cannot carry custom tags. nodes.tf adds a tagged NodeClass + NodePool.
  compute_config {
    enabled    = true
    node_pools = []
  }

  kubernetes_network_config {
    elastic_load_balancing {
      enabled = true
    }
  }

  storage_config {
    block_storage {
      enabled = true
    }
  }

  vpc_config {
    subnet_ids              = module.vpc.private_subnets
    endpoint_private_access = true
    endpoint_public_access  = true
  }

  # No extended-support charges on a demo cluster that outlives its version.
  upgrade_policy {
    support_type = "STANDARD"
  }

  depends_on = [
    aws_iam_role_policy_attachment.cluster,
    aws_iam_role_policy.cluster_custom_tags,
  ]
}

# ------------------------------------------------------------------- cluster admin
# The principal running `terraform apply` (the IAM role behind an assumed-role session).
# Created here rather than by bootstrap_cluster_creator_admin_permissions so that it carries
# the contract tags; the same pattern terraform-aws-modules/eks uses.

data "aws_iam_session_context" "current" {
  arn = data.aws_caller_identity.current.arn
}

resource "aws_eks_access_entry" "admin" {
  cluster_name  = aws_eks_cluster.hub.name
  principal_arn = data.aws_iam_session_context.current.issuer_arn
  type          = "STANDARD"
}

resource "aws_eks_access_policy_association" "admin" {
  cluster_name  = aws_eks_cluster.hub.name
  principal_arn = aws_eks_access_entry.admin.principal_arn
  policy_arn    = "arn:${data.aws_partition.current.partition}:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy"

  access_scope {
    type = "cluster"
  }
}

# A custom NodeClass needs an EC2 access entry for its node role (EKS only creates one for
# the built-in node pools).
resource "aws_eks_access_entry" "node" {
  cluster_name  = aws_eks_cluster.hub.name
  principal_arn = aws_iam_role.node.arn
  type          = "EC2"
}

resource "aws_eks_access_policy_association" "node" {
  cluster_name  = aws_eks_cluster.hub.name
  principal_arn = aws_iam_role.node.arn
  policy_arn    = "arn:${data.aws_partition.current.partition}:eks::aws:cluster-access-policy/AmazonEKSAutoNodePolicy"

  access_scope {
    type = "cluster"
  }

  depends_on = [aws_eks_access_entry.node]
}

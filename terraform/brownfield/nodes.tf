# A tagged NodeClass and a NodePool equivalent to the built-in general-purpose pool.
#
# Why not the built-in pools: they use the EKS-managed "default" NodeClass, and EKS
# reverts any edit to it, so the instances, root volumes and ENIs it launches can never
# carry auto-delete=no. This NodeClass sets spec.tags, which the custom-tags statement on
# the cluster role (eks.tf) allows.
#
# Why a local Helm chart instead of kubernetes_manifest: kubernetes_manifest reads the
# CRD schema from the API server at plan time, which fails on the first apply when the
# cluster does not exist yet. helm_release does not.

resource "helm_release" "nodes" {
  name      = "encabulator-nodes"
  chart     = "${path.module}/charts/auto-mode-nodes"
  namespace = "kube-system"

  values = [yamlencode({
    name             = local.namespace
    nodeRole         = aws_iam_role.node.name
    subnetIds        = module.vpc.private_subnets
    securityGroupIds = [aws_eks_cluster.hub.vpc_config[0].cluster_security_group_id]
    tags             = local.tags
    # Same shape as the built-in general-purpose pool: C/M/R, generation 5+, amd64,
    # on-demand.
    instanceCategories = ["c", "m", "r"]
    instanceGeneration = 4
    architectures      = ["amd64"]
    cpuLimit           = 64
  })]

  depends_on = [
    aws_eks_access_policy_association.node,
    aws_iam_role_policy_attachment.node,
  ]
}

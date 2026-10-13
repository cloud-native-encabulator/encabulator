# The tagger: a CronJob on the hub that puts the three contract tags on what AWS creates
# implicitly, in the hub and in every cell (contract D10). tagger/sweep.py says exactly what.
#
# Everything this stack, ACK and kro create on purpose is tagged when it is created: provider
# default_tags, ACK spec.tags, the NodeClass tags, the StorageClass tagSpecification_N. What is
# left is what AWS creates on its own, without a tag request:
#   - in a cell's VPC: the main route table, default security group and default network ACL
#     (on the hub the VPC module manages those three, so default_tags reaches them, see
#     network.tf), and the cluster security group EKS creates (on the hub: aws_ec2_tag in
#     network.tf);
#   - in every VPC: the requester-managed network interfaces of the EKS control plane, the NAT
#     gateway, RDS and ElastiCache;
#   - ElastiCache serverless snapshots and the instance profiles EKS Auto Mode creates.
#
# A CronJob and not a one-time pass: the requester-managed interfaces appear whenever those
# services create or replace them (RDS and ElastiCache on a cell only after Argo CD has synced
# the app there), and a cell is built long after this stack is applied. It runs on the hub
# because the hub is where the cells come from; ACK cannot give pods on the hub an AWS identity
# (the hub cluster is outside its permissions on purpose), so the role and the Pod Identity
# association are here.

locals {
  tagger_name = "encabulator-tagger"

  # Python 3.13.16 with boto3 1.42.97 (AWS Lambda base image; only python3 and boto3 are used).
  tagger_image = "public.ecr.aws/lambda/python:3.13@sha256:40dcef2531da40c0fae6d11ee28e12313af9723fc1f0f31c1728c22e5112ec3c"

  # Auto Mode creates an instance profile for each NodeClass role: the hub's and every cell's.
  tagger_node_roles = concat([aws_iam_role.node.name], [for name in var.ack_eks_cluster_names : "${name}-node"])
}

# ------------------------------------------------------------------------- IAM role

data "aws_iam_policy_document" "tagger_assume" {
  statement {
    actions = ["sts:AssumeRole", "sts:TagSession"]
    principals {
      type        = "Service"
      identifiers = ["pods.eks.amazonaws.com"]
    }
    # Only the tagger's service account on the hub (EKS Pod Identity session tags).
    condition {
      test     = "StringEquals"
      variable = "aws:RequestTag/eks-cluster-name"
      values   = [local.cluster_name]
    }
    condition {
      test     = "StringEquals"
      variable = "aws:RequestTag/kubernetes-namespace"
      values   = [local.tagger_name]
    }
    condition {
      test     = "StringEquals"
      variable = "aws:RequestTag/kubernetes-service-account"
      values   = [local.tagger_name]
    }
  }
}

resource "aws_iam_role" "tagger" {
  name               = "${local.cluster_name}-tagger"
  description        = "Adds the contract tags to resources AWS creates implicitly for ${local.cluster_name} and its cells"
  assume_role_policy = data.aws_iam_policy_document.tagger_assume.json
}

# Reads, plus tag writes that can only ever set auto-delete=no, project=encabulator and
# talk=opn315: any other key, or any other value, is denied.
data "aws_iam_policy_document" "tagger" {
  statement {
    sid = "Read"
    actions = [
      "ec2:DescribeVpcs",
      "ec2:DescribeRouteTables",
      "ec2:DescribeNetworkAcls",
      "ec2:DescribeSecurityGroups",
      "ec2:DescribeNetworkInterfaces",
      "elasticache:DescribeServerlessCacheSnapshots",
      "elasticache:ListTagsForResource",
    ]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "aws:RequestedRegion"
      values   = var.ack_regions
    }
  }

  statement {
    sid     = "TagVpcResources"
    actions = ["ec2:CreateTags"]
    resources = [
      for type in ["route-table", "network-acl", "security-group", "network-interface"] :
      "arn:${local.partition}:ec2:*:${local.account_id}:${type}/*"
    ]
    condition {
      test     = "StringEquals"
      variable = "aws:RequestedRegion"
      values   = var.ack_regions
    }
    condition {
      test     = "ForAllValues:StringEquals"
      variable = "aws:TagKeys"
      values   = keys(local.tags)
    }
    dynamic "condition" {
      for_each = local.tags
      content {
        test     = "StringEquals"
        variable = "aws:RequestTag/${condition.key}"
        values   = [condition.value]
      }
    }
  }

  statement {
    sid       = "TagCacheSnapshots"
    actions   = ["elasticache:AddTagsToResource"]
    resources = ["arn:${local.partition}:elasticache:*:${local.account_id}:serverlesscachesnapshot:*"]
    condition {
      test     = "StringEquals"
      variable = "aws:RequestedRegion"
      values   = var.ack_regions
    }
    condition {
      test     = "ForAllValues:StringEquals"
      variable = "aws:TagKeys"
      values   = keys(local.tags)
    }
    dynamic "condition" {
      for_each = local.tags
      content {
        test     = "StringEquals"
        variable = "aws:RequestTag/${condition.key}"
        values   = [condition.value]
      }
    }
  }

  statement {
    sid       = "ReadNodeInstanceProfiles"
    actions   = ["iam:ListInstanceProfilesForRole"]
    resources = [for role in local.tagger_node_roles : "arn:${local.partition}:iam::${local.account_id}:role/${role}"]
  }

  # Auto Mode's instance profiles are named eks* (AmazonEKSComputePolicy).
  statement {
    sid       = "ReadInstanceProfileTags"
    actions   = ["iam:ListInstanceProfileTags"]
    resources = ["arn:${local.partition}:iam::${local.account_id}:instance-profile/eks*"]
  }

  statement {
    sid       = "TagInstanceProfiles"
    actions   = ["iam:TagInstanceProfile"]
    resources = ["arn:${local.partition}:iam::${local.account_id}:instance-profile/eks*"]
    condition {
      test     = "ForAllValues:StringEquals"
      variable = "aws:TagKeys"
      values   = keys(local.tags)
    }
    dynamic "condition" {
      for_each = local.tags
      content {
        test     = "StringEquals"
        variable = "aws:RequestTag/${condition.key}"
        values   = [condition.value]
      }
    }
  }
}

resource "aws_iam_role_policy" "tagger" {
  name   = "tag-implicit-resources"
  role   = aws_iam_role.tagger.id
  policy = data.aws_iam_policy_document.tagger.json
}

# EKS Auto Mode runs the Pod Identity agent itself.
resource "aws_eks_pod_identity_association" "tagger" {
  cluster_name    = aws_eks_cluster.hub.name
  namespace       = local.tagger_name
  service_account = local.tagger_name
  role_arn        = aws_iam_role.tagger.arn
}

# ----------------------------------------------------------------------- Kubernetes

resource "kubernetes_namespace_v1" "tagger" {
  metadata {
    name = local.tagger_name
  }

  # Auto Mode needs nodes before anything in the namespace can run.
  depends_on = [helm_release.nodes]
}

resource "kubernetes_service_account_v1" "tagger" {
  metadata {
    name      = local.tagger_name
    namespace = kubernetes_namespace_v1.tagger.metadata[0].name
  }
  # The tagger calls AWS only; it never talks to the Kubernetes API.
  automount_service_account_token = false
}

resource "kubernetes_config_map_v1" "tagger" {
  metadata {
    name      = local.tagger_name
    namespace = kubernetes_namespace_v1.tagger.metadata[0].name
  }

  data = {
    "sweep.py" = file("${path.module}/tagger/sweep.py")
  }
}

resource "kubernetes_cron_job_v1" "tagger" {
  metadata {
    name      = local.tagger_name
    namespace = kubernetes_namespace_v1.tagger.metadata[0].name
    labels = {
      "app.kubernetes.io/name"    = local.tagger_name
      "app.kubernetes.io/part-of" = "encabulator"
    }
  }

  spec {
    schedule                      = "*/10 * * * *"
    concurrency_policy            = "Forbid"
    starting_deadline_seconds     = 300
    successful_jobs_history_limit = 1
    failed_jobs_history_limit     = 3

    job_template {
      metadata {
        labels = {
          "app.kubernetes.io/name"    = local.tagger_name
          "app.kubernetes.io/part-of" = "encabulator"
        }
      }

      spec {
        backoff_limit           = 1
        active_deadline_seconds = 480

        template {
          metadata {
            labels = {
              "app.kubernetes.io/name"    = local.tagger_name
              "app.kubernetes.io/part-of" = "encabulator"
            }
            annotations = {
              "encabulator.io/script-sha256" = sha256(kubernetes_config_map_v1.tagger.data["sweep.py"])
            }
          }

          spec {
            service_account_name             = kubernetes_service_account_v1.tagger.metadata[0].name
            automount_service_account_token  = false
            restart_policy                   = "Never"
            termination_grace_period_seconds = 10

            security_context {
              run_as_non_root = true
              run_as_user     = 65532
              run_as_group    = 65532
              seccomp_profile {
                type = "RuntimeDefault"
              }
            }

            container {
              name    = "sweep"
              image   = local.tagger_image
              command = ["python3", "/scripts/sweep.py"]

              env {
                name  = "REGIONS"
                value = join(" ", var.ack_regions)
              }
              env {
                name  = "CACHE_NAMES"
                value = local.valkey_cache_name
              }
              env {
                name  = "NODE_ROLES"
                value = join(" ", local.tagger_node_roles)
              }
              env {
                name  = "HOME"
                value = "/tmp"
              }

              resources {
                requests = {
                  cpu    = "50m"
                  memory = "128Mi"
                }
                limits = {
                  cpu    = "500m"
                  memory = "256Mi"
                }
              }

              security_context {
                allow_privilege_escalation = false
                read_only_root_filesystem  = true
                capabilities {
                  drop = ["ALL"]
                }
              }

              volume_mount {
                name       = "scripts"
                mount_path = "/scripts"
                read_only  = true
              }
              volume_mount {
                name       = "tmp"
                mount_path = "/tmp"
              }
            }

            volume {
              name = "scripts"
              config_map {
                name = kubernetes_config_map_v1.tagger.metadata[0].name
              }
            }
            volume {
              name = "tmp"
              empty_dir {}
            }
          }
        }
      }
    }
  }

  depends_on = [aws_eks_pod_identity_association.tagger, aws_iam_role_policy.tagger]
}

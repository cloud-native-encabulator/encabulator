# Offline plan test for contract D10: every AWS resource carries auto-delete=no.
#
#   terraform test -filter=tests/tags.tftest.hcl            # the assertions below
#   terraform test -filter=tests/tags.tftest.hcl -verbose   # the plan scripts/tag-check-audit.py reads
#
# Same fake-credential AWS provider and mocked Kubernetes/Helm providers as brownfield.tftest.hcl,
# but no override_resource: an overridden resource plans with an empty tags_all, and the point of
# this run is the real tags_all of every resource (including the VPC module's, which assertions
# cannot reach and the audit script reads from the -verbose plan).

provider "aws" {
  region                      = "us-west-2"
  access_key                  = "mock_access_key"
  secret_key                  = "mock_secret_key"
  skip_credentials_validation = true
  skip_requesting_account_id  = true
  skip_metadata_api_check     = true
  skip_region_validation      = true

  default_tags {
    tags = {
      "auto-delete" = "no"
      "project"     = "encabulator"
      "talk"        = "opn315"
    }
  }
}

mock_provider "kubernetes" {}
mock_provider "helm" {}

override_data {
  target = data.aws_availability_zones.available
  values = {
    names = ["us-west-2a", "us-west-2b", "us-west-2c", "us-west-2d"]
  }
}

override_data {
  target = data.aws_caller_identity.current
  values = {
    account_id = "111122223333"
    arn        = "arn:aws:sts::111122223333:assumed-role/Admin/presenter"
  }
}

override_data {
  target = data.aws_iam_session_context.current
  values = {
    issuer_arn = "arn:aws:iam::111122223333:role/Admin"
  }
}

variables {
  idc_instance_arn = "arn:aws:sso:::instance/ssoins-0123456789abcdef"
}

run "tags" {
  command = plan

  assert {
    condition = alltrue([
      for t in [
        aws_eks_cluster.hub.tags_all,
        aws_security_group.db.tags_all,
        aws_iam_role.cluster.tags_all,
        aws_iam_role.node.tags_all,
        aws_iam_role.tagger.tags_all,
        aws_db_instance.pg.tags_all,
        aws_db_subnet_group.encabulator.tags_all,
        aws_elasticache_serverless_cache.valkey.tags_all,
        aws_eks_pod_identity_association.tagger.tags_all,
      ] : t["auto-delete"] == "no"
    ])
    error_message = "the cluster, the store security group, the IAM roles, the stores and the tagger's association carry auto-delete=no"
  }
  assert {
    condition     = aws_db_instance.pg.copy_tags_to_snapshot
    error_message = "RDS copies the instance tags to its automated backups"
  }

  # ----------------------------------------------------------- EKS access entries
  assert {
    condition     = !aws_eks_cluster.hub.access_config[0].bootstrap_cluster_creator_admin_permissions && aws_eks_access_entry.admin.principal_arn == "arn:aws:iam::111122223333:role/Admin" && aws_eks_access_policy_association.admin.policy_arn == "arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy"
    error_message = "no EKS-created (untagged) creator entry: the applying role gets an explicit cluster-admin access entry"
  }
  assert {
    condition     = toset(keys(aws_eks_access_entry.capability)) == toset(["argocd", "ack"]) && alltrue([for e in aws_eks_access_entry.capability : e.type == "STANDARD"])
    error_message = "Argo CD and ACK access entries are created before their capabilities, like kro's"
  }
  assert {
    condition = alltrue([
      for t in concat(
        [for e in aws_eks_access_entry.capability : e.tags_all],
        [
          aws_eks_access_entry.kro.tags_all,
          aws_eks_access_entry.admin.tags_all,
          aws_eks_access_entry.node.tags_all,
          aws_eks_capability.argocd.tags_all,
          aws_eks_capability.kro.tags_all,
          aws_eks_capability.ack.tags_all,
        ],
      ) : t["auto-delete"] == "no"
    ])
    error_message = "every EKS access entry and capability carries auto-delete=no"
  }

  # ------------------------------------------------------------------ the tagger
  assert {
    condition = alltrue([
      for s in jsondecode(data.aws_iam_policy_document.tagger.json).Statement :
      alltrue([for a in flatten([s.Action]) : can(regex("^(ec2:Describe|elasticache:Describe|elasticache:ListTagsForResource$|iam:List|eks:ListAccessEntries$|eks:DescribeAccessEntry$)", a)) || contains(["ec2:CreateTags", "elasticache:AddTagsToResource", "iam:TagInstanceProfile", "eks:TagResource"], a)])
    ])
    error_message = "the tagger can only read and add tags"
  }
  assert {
    condition = alltrue([
      for s in jsondecode(data.aws_iam_policy_document.tagger.json).Statement :
      alltrue([
        toset(flatten([s.Condition["ForAllValues:StringEquals"]["aws:TagKeys"]])) == toset(["auto-delete", "project", "talk"]),
        s.Condition.StringEquals["aws:RequestTag/auto-delete"] == "no",
        s.Condition.StringEquals["aws:RequestTag/project"] == "encabulator",
        s.Condition.StringEquals["aws:RequestTag/talk"] == "opn315",
      ]) if length(setintersection(toset(flatten([s.Action])), toset(["ec2:CreateTags", "elasticache:AddTagsToResource", "iam:TagInstanceProfile", "eks:TagResource"]))) > 0
    ])
    error_message = "every tag write the tagger may make is limited to exactly auto-delete=no, project=encabulator, talk=opn315"
  }
  assert {
    condition = alltrue([
      for s in jsondecode(data.aws_iam_policy_document.tagger.json).Statement :
      alltrue([
        toset(flatten([s.Action])) == toset(["ec2:CreateTags"]),
        toset(flatten([s.Resource])) == toset([
          for type in ["route-table", "network-acl", "security-group", "network-interface", "vpc-endpoint"] :
          "arn:aws:ec2:*:111122223333:${type}/*"
        ]),
      ]) if contains(flatten([s.Action]), "ec2:CreateTags")
    ])
    error_message = "the tagger may tag exactly route tables, network ACLs, security groups, network interfaces and VPC endpoints in EC2, nothing else"
  }
  assert {
    condition = anytrue([
      for s in jsondecode(data.aws_iam_policy_document.tagger.json).Statement :
      contains(flatten([s.Action]), "ec2:DescribeVpcEndpoints") && s.Resource == "*"
    ])
    error_message = "the tagger may describe VPC endpoints (ElastiCache Serverless creates three per VPC)"
  }
  assert {
    condition = alltrue([
      jsondecode(data.aws_iam_policy_document.tagger_assume.json).Statement[0].Principal.Service == "pods.eks.amazonaws.com",
      jsondecode(data.aws_iam_policy_document.tagger_assume.json).Statement[0].Condition.StringEquals["aws:RequestTag/kubernetes-namespace"] == "encabulator-tagger",
      jsondecode(data.aws_iam_policy_document.tagger_assume.json).Statement[0].Condition.StringEquals["aws:RequestTag/kubernetes-service-account"] == "encabulator-tagger",
      jsondecode(data.aws_iam_policy_document.tagger_assume.json).Statement[0].Condition.StringEquals["aws:RequestTag/eks-cluster-name"] == "encabulator-usw2",
      aws_eks_pod_identity_association.tagger.namespace == "encabulator-tagger",
      aws_eks_pod_identity_association.tagger.service_account == "encabulator-tagger",
    ])
    error_message = "only the tagger's service account on the hub can assume the tagger role (EKS Pod Identity)"
  }
  assert {
    condition = alltrue([
      kubernetes_cron_job_v1.tagger.spec[0].schedule == "*/10 * * * *",
      kubernetes_cron_job_v1.tagger.spec[0].concurrency_policy == "Forbid",
      strcontains(kubernetes_cron_job_v1.tagger.spec[0].job_template[0].spec[0].template[0].spec[0].container[0].image, "@sha256:"),
      kubernetes_config_map_v1.tagger.data["sweep.py"] == file("${path.module}/tagger/sweep.py"),
    ])
    error_message = "the tagger runs sweep.py every 10 minutes from a digest-pinned image"
  }
  assert {
    condition = (
      { for e in kubernetes_cron_job_v1.tagger.spec[0].job_template[0].spec[0].template[0].spec[0].container[0].env : e.name => e.value } ==
      {
        REGIONS       = "us-west-2 us-east-2 eu-west-1"
        CACHE_NAMES   = "encabulator-valkey"
        NODE_ROLES    = "encabulator-usw2-node encabulator-use2-node encabulator-reh1-node encabulator-euw1-node"
        CLUSTER_NAMES = "encabulator-usw2 encabulator-use2 encabulator-reh1 encabulator-euw1"
        HOME          = "/tmp"
      }
    )
    error_message = "the tagger sweeps the hub and cell regions, the Valkey cache's snapshots, every NodeClass role's instance profiles and every cluster's access entries"
  }
}

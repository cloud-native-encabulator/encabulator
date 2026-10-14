# Offline plan test: `terraform test` (no AWS account, no cluster).
#
# The AWS provider is real but has fake credentials and skips every account lookup; the two
# data sources that would call AWS during plan are overridden. The Kubernetes and Helm
# providers are mocked. This proves the whole configuration plans and pins the values the
# other parts of the talk depend on (names, tags, hand-off keys, capability permissions).

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

# Ids that only exist after apply, so the NodeClass values can be checked at plan time.
override_resource {
  target          = aws_eks_cluster.hub
  override_during = plan
  values = {
    arn      = "arn:aws:eks:us-west-2:111122223333:cluster/encabulator-usw2"
    endpoint = "https://EXAMPLE.gr7.us-west-2.eks.amazonaws.com"
    vpc_config = {
      cluster_security_group_id = "sg-0bbbbbbbbbbbbbbb1"
    }
  }
}

override_resource {
  target          = module.vpc.aws_subnet.private[0]
  override_during = plan
  values = {
    id = "subnet-0aaaaaaaaaaaaaaa1"
  }
}

override_resource {
  target          = module.vpc.aws_subnet.private[1]
  override_during = plan
  values = {
    id = "subnet-0aaaaaaaaaaaaaaa2"
  }
}

override_resource {
  target          = module.vpc.aws_subnet.private[2]
  override_during = plan
  values = {
    id = "subnet-0aaaaaaaaaaaaaaa3"
  }
}

override_resource {
  target          = aws_security_group.db
  override_during = plan
  values = {
    id = "sg-0ccccccccccccccc1"
  }
}

variables {
  idc_instance_arn = "arn:aws:sso:::instance/ssoins-0123456789abcdef"
}

run "plan" {
  command = plan

  # ------------------------------------------------------------------- names
  assert {
    condition     = aws_eks_cluster.hub.name == "encabulator-usw2"
    error_message = "hub cluster must be encabulator-usw2"
  }
  assert {
    condition     = aws_db_instance.pg.identifier == "encabulator-pg" && aws_db_subnet_group.encabulator.name == "encabulator" && aws_security_group.db.name == "encabulator-db" && aws_elasticache_serverless_cache.valkey.name == "encabulator-valkey"
    error_message = "store names must match the contract"
  }
  assert {
    condition     = toset([for r in aws_iam_role.capability : r.name]) == toset(["encabulator-usw2-argocd", "encabulator-usw2-ack", "encabulator-usw2-kro"])
    error_message = "capability role names must be encabulator-usw2-argocd/-ack/-kro (D5)"
  }
  assert {
    condition     = length(module.vpc.private_subnets) == 3 && length(local.azs) == 3
    error_message = "three AZs, three private subnets"
  }

  # ------------------------------------------------------------- store values (D3)
  assert {
    condition = alltrue([
      aws_db_instance.pg.engine == "postgres",
      aws_db_instance.pg.engine_version == "18",
      aws_db_instance.pg.instance_class == "db.t4g.medium",
      aws_db_instance.pg.allocated_storage == 20,
      aws_db_instance.pg.storage_type == "gp3",
      aws_db_instance.pg.storage_encrypted,
      !aws_db_instance.pg.publicly_accessible,
      !aws_db_instance.pg.multi_az,
      aws_db_instance.pg.backup_retention_period == 7,
      aws_db_instance.pg.username == "postgres",
      aws_db_instance.pg.max_allocated_storage == null,
      try(length(aws_db_instance.pg.enabled_cloudwatch_logs_exports), 0) == 0,
      aws_db_instance.pg.performance_insights_enabled != true,
    ])
    error_message = "RDS values must match the contract and D3"
  }
  assert {
    condition = alltrue([
      aws_elasticache_serverless_cache.valkey.engine == "valkey",
      aws_elasticache_serverless_cache.valkey.major_engine_version == "8",
      aws_elasticache_serverless_cache.valkey.description == "Encabulator Valkey cache",
      aws_elasticache_serverless_cache.valkey.daily_snapshot_time == "09:00",
      aws_elasticache_serverless_cache.valkey.snapshot_retention_limit == 1,
      aws_db_subnet_group.encabulator.description == "Encabulator databases",
    ])
    error_message = "ElastiCache and subnet group values must match D3"
  }
  assert {
    condition = alltrue([
      aws_elasticache_serverless_cache.valkey.subnet_ids == aws_db_subnet_group.encabulator.subnet_ids,
      length(aws_db_subnet_group.encabulator.subnet_ids) == 3,
      aws_elasticache_serverless_cache.valkey.security_group_ids == toset(["sg-0ccccccccccccccc1"]),
      aws_db_instance.pg.vpc_security_group_ids == toset(["sg-0ccccccccccccccc1"]),
    ])
    error_message = "cache uses the DB subnet group's subnets and encabulator-db as its only security group (register-hub.sh checks this)"
  }
  assert {
    condition = alltrue([
      for t in [
        aws_db_instance.pg.tags_all,
        aws_elasticache_serverless_cache.valkey.tags_all,
        aws_db_subnet_group.encabulator.tags_all,
      ] : t == tomap({ "auto-delete" = "no", "project" = "encabulator", "talk" = "opn315" })
    ])
    error_message = "adopted stores carry exactly the three contract tags (D3)"
  }
  assert {
    condition     = aws_iam_role.cluster.tags_all["auto-delete"] == "no" && aws_vpc_security_group_ingress_rule.db["postgres"].tags_all["auto-delete"] == "no" && aws_eks_capability.ack.tags_all["auto-delete"] == "no"
    error_message = "other resources carry auto-delete=no"
  }
  assert {
    condition     = toset(keys(aws_ec2_tag.cluster_security_group)) == toset(["auto-delete", "project", "talk"])
    error_message = "the EKS-created cluster security group is tagged too"
  }

  # ---------------------------------------------------------------- Auto Mode (D8)
  assert {
    condition     = aws_eks_cluster.hub.compute_config[0].enabled && try(length(aws_eks_cluster.hub.compute_config[0].node_pools), 0) == 0
    error_message = "Auto Mode on with the built-in node pools off"
  }
  assert {
    condition = alltrue([
      yamldecode(helm_release.nodes.values[0]).tags == { "auto-delete" = "no", "project" = "encabulator", "talk" = "opn315" },
      yamldecode(helm_release.nodes.values[0]).nodeRole == "encabulator-usw2-node",
      yamldecode(helm_release.nodes.values[0]).securityGroupIds == ["sg-0bbbbbbbbbbbbbbb1"],
      length(yamldecode(helm_release.nodes.values[0]).subnetIds) == 3,
    ])
    error_message = "the custom NodeClass carries the three tags, the node role and the cluster security group"
  }
  assert {
    condition     = aws_eks_access_entry.node.type == "EC2"
    error_message = "the custom NodeClass role needs an EC2 access entry"
  }

  # ---------------------------------------------------------- hand-off objects
  assert {
    condition     = toset(keys(kubernetes_config_map_v1.aws.data)) == toset(["POSTGRES_HOST", "POSTGRES_PORT", "VALKEY_HOST", "VALKEY_PORT"])
    error_message = "ConfigMap encabulator-aws keys"
  }
  assert {
    condition     = kubernetes_config_map_v1.aws.metadata[0].name == "encabulator-aws" && kubernetes_config_map_v1.aws.metadata[0].namespace == "encabulator"
    error_message = "ConfigMap name and namespace"
  }
  assert {
    condition     = kubernetes_secret_v1.db.metadata[0].name == "encabulator-db" && toset(keys(kubernetes_secret_v1.db.data)) == toset(["postgres-password", "hydrocoptic-password", "detractor-password", "girdlespring-password"])
    error_message = "Secret encabulator-db keys"
  }
  assert {
    condition     = kubernetes_secret_v1.valkey.metadata[0].name == "encabulator-valkey" && kubernetes_secret_v1.valkey.data == tomap({ "valkey-password" = "" })
    error_message = "Secret encabulator-valkey holds one empty valkey-password key"
  }

  # ----------------------------------------------------------- capabilities (D4, D5)
  assert {
    condition     = aws_eks_access_entry.kro.kubernetes_groups == toset(["encabulator:kro"])
    error_message = "kro access entry carries group encabulator:kro"
  }
  assert {
    condition     = aws_eks_capability.argocd.configuration[0].argo_cd[0].namespace == "argocd"
    error_message = "Argo CD namespace argocd"
  }
  assert {
    condition     = length(aws_eks_access_policy_association.argocd_cluster_admin) == 1 && length(aws_eks_access_policy_association.kro_cluster_admin) == 0
    error_message = "defaults: Argo CD cluster admin (demo only), kro via group RBAC"
  }
  assert {
    condition = alltrue([
      for s in jsondecode(data.aws_iam_policy_document.ack.json).Statement :
      alltrue([for r in flatten([s.Resource]) : !strcontains(r, "encabulator-usw2")])
    ])
    error_message = "the ACK policy never names the hub's own cluster or roles"
  }
  assert {
    condition = alltrue([
      for s in jsondecode(data.aws_iam_policy_document.ack.json).Statement :
      try(toset(s.Condition.StringEquals["aws:RequestedRegion"]) == toset(["us-west-2", "us-east-2", "eu-west-1"]), true)
    ])
    error_message = "ACK region scope is the hub (us-west-2) and the cell regions (us-east-2, eu-west-1)"
  }
  assert {
    condition = alltrue([
      for p in ["encabulator-use2-*", "encabulator-reh1-*", "encabulator-euw1-*"] : contains(
        flatten([for s in jsondecode(data.aws_iam_policy_document.ack.json).Statement : flatten([s.Resource]) if s.Sid == "IamPassCellRoles"]),
        "arn:aws:iam::111122223333:role/${p}",
      )
    ])
    error_message = "ACK may pass the roles of every cell (encabulator-use2-*, -reh1-*, -euw1-*)"
  }
}

# Act 1 hand-off, shipped DISABLED. Terraform ignores this file until it is renamed.
#
# Run it only after ACK has adopted the three resources (gitops/hub/brownfield) and their
# ACK resources report ACK.ResourceSynced=True:
#
#   git rm stores.tf
#   git mv handoff.tf.example handoff.tf
#   terraform plan     # 3 resources "will no longer be managed by Terraform", nothing else
#   terraform apply
#
# destroy = false means Terraform drops them from state and leaves them running. After the
# apply, `terraform plan` is clean: the ConfigMap is built from data sources that look the
# stores up by name, and the security group and VPC are still Terraform's.

removed {
  from = aws_db_instance.pg

  lifecycle {
    destroy = false
  }
}

removed {
  from = aws_elasticache_serverless_cache.valkey

  lifecycle {
    destroy = false
  }
}

removed {
  from = aws_db_subnet_group.encabulator

  lifecycle {
    destroy = false
  }
}

# Replaces the locals block at the bottom of stores.tf: the stores are now looked up by
# their fixed names.
locals {
  postgres_lookup_identifier = local.db_identifier
  valkey_lookup_name         = local.valkey_cache_name
}

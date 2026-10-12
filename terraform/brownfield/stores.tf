# The three resources ACK adopts in Act 1. During the hand-off this whole file is deleted
# and handoff.tf.example becomes handoff.tf: its removed {} blocks tell Terraform to forget
# them without destroying anything, and its locals replace the ones at the bottom here so
# the data sources in kubernetes.tf keep working. The security group and the VPC are not
# in this file: they stay with Terraform.

#
# ACK reads every value back after adoption and treats a difference from the ACK resource
# in gitops/hub/brownfield as drift. So the values here are exactly the shared contract's
# (lead decision D3): engine major 18, no max_allocated_storage, no log exports, no
# Performance Insights, and only the three default tags (no Name tag). Change one here and
# the matching ACK resource has to change with it.

resource "aws_db_subnet_group" "encabulator" {
  name        = local.db_subnet_group
  description = "Encabulator databases"
  subnet_ids  = module.vpc.private_subnets
}

resource "aws_db_instance" "pg" {
  identifier     = local.db_identifier
  engine         = "postgres"
  engine_version = var.postgres_engine_version
  instance_class = var.db_instance_class
  port           = 5432

  allocated_storage = 20
  storage_type      = "gp3"
  storage_encrypted = true

  username = "postgres"
  password = random_password.postgres.result

  db_subnet_group_name   = aws_db_subnet_group.encabulator.name
  vpc_security_group_ids = [aws_security_group.db.id]
  publicly_accessible    = false
  multi_az               = false

  backup_retention_period    = 7
  copy_tags_to_snapshot      = true
  auto_minor_version_upgrade = true
  apply_immediately          = true

  # Demo settings: no final snapshot and no deletion protection, so teardown is one step.
  skip_final_snapshot = true
  deletion_protection = false
}

# Serverless caches always encrypt in transit (TLS). No user group: access is controlled
# by the security group only, and clients connect without a password.
#
# Same subnets as the DB subnet group and encabulator-db as the only security group:
# gitops/hub/bootstrap/register-hub.sh reads the ids from those two and refuses to
# register the hub when the cache disagrees.
resource "aws_elasticache_serverless_cache" "valkey" {
  name                     = local.valkey_cache_name
  description              = "Encabulator Valkey cache"
  engine                   = "valkey"
  major_engine_version     = var.valkey_major_engine_version
  subnet_ids               = aws_db_subnet_group.encabulator.subnet_ids
  security_group_ids       = [aws_security_group.db.id]
  daily_snapshot_time      = "09:00"
  snapshot_retention_limit = 1
}

# Read by the data sources in kubernetes.tf. The names are taken from the resources' ARNs
# on purpose: an ARN is unknown until the resource exists, so on the first apply Terraform
# reads those data sources after the stores are created (a literal name would make it look
# them up at plan time and fail). handoff.tf.example carries the literal-name version.
locals {
  # arn:aws:rds:<region>:<account>:db:encabulator-pg
  postgres_lookup_identifier = element(split(":", aws_db_instance.pg.arn), 6)
  # arn:aws:elasticache:<region>:<account>:serverlesscache:encabulator-valkey
  valkey_lookup_name = element(split(":", aws_elasticache_serverless_cache.valkey.arn), 6)
}

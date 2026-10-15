# The Act 1 "prepared earlier" starting point: what a team built with Terraform before the
# talk. VPC, hub EKS Auto Mode cluster, RDS for PostgreSQL, ElastiCache serverless Valkey,
# the EKS capabilities (Argo CD, ACK, kro), and a ConfigMap + Secret hand-off that the
# Helm chart reads. Names come from the shared OPN315 contract.

locals {
  cluster_name = "encabulator-usw2"
  namespace    = "encabulator"

  # Stores. ACK adopts these three in Act 1 (handoff.tf.example).
  db_identifier     = "encabulator-pg"
  db_subnet_group   = "encabulator"
  valkey_cache_name = "encabulator-valkey"

  # Stays with Terraform.
  db_security_group = "encabulator-db"

  # One logical database and one role (same name) per owning service.
  databases = ["hydrocoptic", "detractor", "girdlespring"]

  tags = {
    "auto-delete" = "no"
    "project"     = "encabulator"
    "talk"        = "opn315"
  }

  azs = slice(data.aws_availability_zones.available.names, 0, 3)
}

data "aws_availability_zones" "available" {
  state = "available"

  # Regular zones only; Local Zones and Wavelength Zones are opt-in.
  filter {
    name   = "opt-in-status"
    values = ["opt-in-not-required"]
  }
}

data "aws_caller_identity" "current" {}

data "aws_partition" "current" {}

# VPC across three AZs: private subnets for nodes, RDS and ElastiCache; public subnets for
# the NAT gateway and internet-facing load balancers. One NAT gateway keeps the demo
# cheap; use one per AZ (single_nat_gateway = false) for anything that must survive an AZ.

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "6.7.3"

  name = local.cluster_name
  cidr = var.vpc_cidr
  azs  = local.azs

  # /19 per private subnet: Auto Mode hands out /28 prefixes per node.
  private_subnets = [for i in range(3) : cidrsubnet(var.vpc_cidr, 3, i)]
  public_subnets  = [for i in range(3) : cidrsubnet(var.vpc_cidr, 8, 200 + i)]

  enable_nat_gateway   = true
  single_nat_gateway   = true
  enable_dns_hostnames = true
  enable_dns_support   = true

  # Load balancer subnet discovery for EKS Auto Mode.
  public_subnet_tags = {
    "kubernetes.io/role/elb" = "1"
  }
  private_subnet_tags = {
    "kubernetes.io/role/internal-elb" = "1"
  }
}

# ------------------------------------------------------------- store security group
# Stays with Terraform after the Act 1 hand-off: ACK adopts the stores, which keep
# referring to this group by id.

resource "aws_security_group" "db" {
  name        = local.db_security_group
  description = "Encabulator stores: PostgreSQL and Valkey from the EKS cluster security group"
  vpc_id      = module.vpc.vpc_id

  tags = {
    Name = local.db_security_group
  }
}

resource "aws_vpc_security_group_ingress_rule" "db" {
  for_each = {
    postgres = 5432
    valkey   = 6379
  }

  security_group_id            = aws_security_group.db.id
  referenced_security_group_id = aws_eks_cluster.hub.vpc_config[0].cluster_security_group_id
  ip_protocol                  = "tcp"
  from_port                    = each.value
  to_port                      = each.value
  description                  = "${each.key} from the ${local.cluster_name} cluster security group"
}

# The cluster security group is created by EKS, not by this stack, so default_tags never
# reach it. Tag it explicitly.
resource "aws_ec2_tag" "cluster_security_group" {
  for_each = local.tags

  resource_id = aws_eks_cluster.hub.vpc_config[0].cluster_security_group_id
  key         = each.key
  value       = each.value
}

# Outputs read by the README steps, deploy-app.sh and the GitOps registration scripts.
# Store values come from the data sources so these keep working after the Act 1 hand-off.

output "cluster_name" {
  value = aws_eks_cluster.hub.name
}

output "cluster_arn" {
  description = "Argo CD cluster Secrets use the ARN as `server`."
  value       = aws_eks_cluster.hub.arn
}

output "region" {
  value = var.region
}

output "kubeconfig_command" {
  value = "aws eks update-kubeconfig --region ${var.region} --name ${aws_eks_cluster.hub.name}"
}

output "vpc_id" {
  value = module.vpc.vpc_id
}

output "private_subnet_ids" {
  value = module.vpc.private_subnets
}

output "db_security_group_id" {
  value = aws_security_group.db.id
}

output "cluster_security_group_id" {
  value = aws_eks_cluster.hub.vpc_config[0].cluster_security_group_id
}

output "postgres_host" {
  value = data.aws_db_instance.pg.address
}

output "valkey_host" {
  value = data.aws_elasticache_serverless_cache.valkey.endpoint.address
}

output "capability_role_arns" {
  value = { for k, r in aws_iam_role.capability : k => r.arn }
}

output "argocd_server_url" {
  value = try(aws_eks_capability.argocd.configuration[0].argo_cd[0].server_url, null)
}

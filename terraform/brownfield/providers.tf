provider "aws" {
  region = var.region

  # Every resource this stack creates directly carries these three tags. Resources that
  # EKS Auto Mode creates on the cluster's behalf (instances, volumes, ENIs) get the same
  # tags from the NodeClass and the StorageClass, see nodes.tf and values-brownfield.yaml.
  default_tags {
    tags = local.tags
  }
}

# The Kubernetes and Helm providers talk to the hub cluster this stack creates. They use
# the AWS CLI for a fresh token on every call (a data-source token expires after 15
# minutes, and RDS alone takes longer than that to create).
provider "kubernetes" {
  host                   = aws_eks_cluster.hub.endpoint
  cluster_ca_certificate = base64decode(aws_eks_cluster.hub.certificate_authority[0].data)

  exec {
    api_version = "client.authentication.k8s.io/v1beta1"
    command     = "aws"
    args        = ["eks", "get-token", "--cluster-name", aws_eks_cluster.hub.name, "--region", var.region]
  }
}

provider "helm" {
  kubernetes = {
    host                   = aws_eks_cluster.hub.endpoint
    cluster_ca_certificate = base64decode(aws_eks_cluster.hub.certificate_authority[0].data)

    exec = {
      api_version = "client.authentication.k8s.io/v1beta1"
      command     = "aws"
      args        = ["eks", "get-token", "--cluster-name", aws_eks_cluster.hub.name, "--region", var.region]
    }
  }
}

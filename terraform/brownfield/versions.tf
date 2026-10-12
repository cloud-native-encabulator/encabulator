# Pinned versions. Every one of them is listed in README.md; bump them there too.
#
# `removed { lifecycle { destroy = false } }` (the Act 1 hand-off in handoff.tf.example)
# needs Terraform 1.7 or newer. Tested with 1.14.9.

terraform {
  required_version = ">= 1.7.0, < 2.0.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "= 6.68.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "= 3.3.0"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "= 3.3.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "= 3.9.1"
    }
  }
}

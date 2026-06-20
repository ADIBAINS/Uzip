terraform {
  required_version = ">= 1.8"
  required_providers {
    aws        = { source = "hashicorp/aws", version = "~> 5.0" }
    random     = { source = "hashicorp/random", version = "~> 3.6" }
    kubernetes = { source = "hashicorp/kubernetes", version = "~> 2.36" }
    helm       = { source = "hashicorp/helm", version = "~> 2.17" }
  }
}

provider "aws" { region = var.region }

data "aws_eks_cluster_auth" "main" { name = module.eks.cluster_name }

provider "kubernetes" {
  host                   = module.eks.cluster_endpoint
  cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)
  token                  = data.aws_eks_cluster_auth.main.token
}

provider "helm" {
  kubernetes {
    host                   = module.eks.cluster_endpoint
    cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)
    token                  = data.aws_eks_cluster_auth.main.token
  }
}

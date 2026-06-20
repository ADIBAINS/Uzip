data "aws_availability_zones" "available" { state = "available" }

module "vpc" {
  source               = "terraform-aws-modules/vpc/aws"
  version              = "5.21.0"
  name                 = local.name
  cidr                 = "10.20.0.0/16"
  azs                  = slice(data.aws_availability_zones.available.names, 0, 3)
  private_subnets      = ["10.20.0.0/20", "10.20.16.0/20", "10.20.32.0/20"]
  public_subnets       = ["10.20.128.0/24", "10.20.129.0/24", "10.20.130.0/24"]
  enable_nat_gateway   = true
  single_nat_gateway   = var.environment != "production"
  enable_dns_hostnames = true
  public_subnet_tags   = { "kubernetes.io/role/elb" = 1 }
  private_subnet_tags  = { "kubernetes.io/role/internal-elb" = 1 }
  tags                 = local.tags
}

module "eks" {
  source                         = "terraform-aws-modules/eks/aws"
  version                        = "20.37.2"
  cluster_name                   = local.name
  cluster_version                = "1.36"
  cluster_endpoint_public_access = true
  vpc_id                         = module.vpc.vpc_id
  subnet_ids                     = module.vpc.private_subnets
  enable_irsa                    = true
  eks_managed_node_groups = {
    default = {
      instance_types = ["m7i.large"]
      min_size       = var.environment == "production" ? 3 : 2
      max_size       = var.environment == "production" ? 10 : 4
      desired_size   = var.environment == "production" ? 3 : 2
    }
  }
  tags = local.tags
}

resource "aws_db_subnet_group" "main" {
  name       = local.name
  subnet_ids = module.vpc.private_subnets
  tags       = local.tags
}

resource "aws_security_group" "data" {
  name   = "${local.name}-data"
  vpc_id = module.vpc.vpc_id
  ingress {
    from_port   = 5432
    to_port     = 5432
    protocol    = "tcp"
    cidr_blocks = [module.vpc.vpc_cidr_block]
  }
  ingress {
    from_port   = 6379
    to_port     = 6379
    protocol    = "tcp"
    cidr_blocks = [module.vpc.vpc_cidr_block]
  }
  ingress {
    from_port   = 9098
    to_port     = 9098
    protocol    = "tcp"
    cidr_blocks = [module.vpc.vpc_cidr_block]
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
  tags = local.tags
}

resource "random_password" "db" {
  length  = 32
  special = false
}

resource "random_password" "app_db" {
  length  = 32
  special = false
}

resource "random_password" "clickhouse" {
  length  = 32
  special = false
}

resource "aws_db_instance" "postgres" {
  identifier              = local.name
  engine                  = "postgres"
  engine_version          = "17"
  instance_class          = var.db_instance_class
  allocated_storage       = 20
  max_allocated_storage   = 100
  db_name                 = "uzip"
  username                = "uzip"
  password                = random_password.db.result
  db_subnet_group_name    = aws_db_subnet_group.main.name
  vpc_security_group_ids  = [aws_security_group.data.id]
  storage_encrypted       = true
  multi_az                = var.environment == "production"
  backup_retention_period = var.environment == "production" ? 14 : 3
  deletion_protection     = var.environment == "production"
  skip_final_snapshot     = var.environment != "production"
  tags                    = local.tags
}

resource "aws_elasticache_subnet_group" "main" {
  name       = local.name
  subnet_ids = module.vpc.private_subnets
}

resource "aws_elasticache_replication_group" "redis" {
  replication_group_id       = local.name
  description                = "Uzip redirect cache"
  node_type                  = "cache.t4g.micro"
  port                       = 6379
  subnet_group_name          = aws_elasticache_subnet_group.main.name
  security_group_ids         = [aws_security_group.data.id]
  num_cache_clusters         = var.environment == "production" ? 2 : 1
  automatic_failover_enabled = var.environment == "production"
  at_rest_encryption_enabled = true
  transit_encryption_enabled = true
  tags                       = local.tags
}

resource "aws_msk_serverless_cluster" "kafka" {
  cluster_name = local.name
  vpc_config {
    subnet_ids         = module.vpc.private_subnets
    security_group_ids = [aws_security_group.data.id]
  }
  client_authentication {
    sasl {
      iam {
        enabled = true
      }
    }
  }
  tags = local.tags
}

resource "aws_cognito_user_pool" "main" {
  name                     = local.name
  username_attributes      = ["email"]
  auto_verified_attributes = ["email"]
  schema {
    name                = "tenant_id"
    attribute_data_type = "String"
    mutable             = true
    required            = false
  }
  schema {
    name                = "role"
    attribute_data_type = "String"
    mutable             = true
    required            = false
  }
  tags = local.tags
}

resource "aws_cognito_user_pool_client" "dashboard" {
  name                                 = "${local.name}-dashboard"
  user_pool_id                         = aws_cognito_user_pool.main.id
  generate_secret                      = false
  allowed_oauth_flows_user_pool_client = true
  allowed_oauth_flows                  = ["code"]
  allowed_oauth_scopes                 = ["openid", "email", "profile"]
  supported_identity_providers         = ["COGNITO"]
  callback_urls = compact([
    "http://localhost:3000/auth/callback",
    var.domain_name == "" ? "" : "https://${local.app_domain}/auth/callback"
  ])
  logout_urls = compact([
    "http://localhost:3000",
    var.domain_name == "" ? "" : "https://${local.app_domain}"
  ])
  prevent_user_existence_errors = "ENABLED"
  enable_token_revocation       = true
}

resource "aws_ecr_repository" "services" {
  for_each = toset(["control-api", "redirect", "event-processor", "analytics-api", "dashboard", "topic-init"])
  name     = "${var.project}-${each.value}"
  image_scanning_configuration { scan_on_push = true }
  encryption_configuration { encryption_type = "AES256" }
  tags = local.tags
}

resource "aws_ecr_lifecycle_policy" "services" {
  for_each   = aws_ecr_repository.services
  repository = each.value.name
  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Retain the latest 30 images"
      selection = {
        tagStatus   = "any"
        countType   = "imageCountMoreThan"
        countNumber = 30
      }
      action = { type = "expire" }
    }]
  })
}

data "aws_iam_policy_document" "workload_assume" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = [module.eks.oidc_provider_arn]
    }
    condition {
      test     = "StringEquals"
      variable = "${replace(module.eks.cluster_oidc_issuer_url, "https://", "")}:sub"
      values = [
        "system:serviceaccount:uzip-staging:uzip",
        "system:serviceaccount:uzip-production:uzip"
      ]
    }
  }
}

resource "aws_iam_role" "workload" {
  name               = "${local.name}-workload"
  assume_role_policy = data.aws_iam_policy_document.workload_assume.json
  tags               = local.tags
}

resource "aws_iam_role_policy" "workload" {
  name = "uzip-data-access"
  role = aws_iam_role.workload.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "kafka-cluster:Connect",
          "kafka-cluster:DescribeTopic",
          "kafka-cluster:CreateTopic",
          "kafka-cluster:ReadData",
          "kafka-cluster:WriteData",
          "kafka-cluster:DescribeGroup",
          "kafka-cluster:AlterGroup"
        ]
        Resource = "*"
      },
      {
        Effect   = "Allow"
        Action   = ["secretsmanager:GetSecretValue"]
        Resource = aws_secretsmanager_secret.application.arn
      },
      {
        Effect   = "Allow"
        Action   = ["s3:ListBucket"]
        Resource = aws_s3_bucket.clickhouse_backups.arn
      },
      {
        Effect   = "Allow"
        Action   = ["s3:PutObject", "s3:GetObject", "s3:DeleteObject"]
        Resource = "${aws_s3_bucket.clickhouse_backups.arn}/*"
      }
    ]
  })
}

resource "aws_secretsmanager_secret" "application" {
  name                    = "${var.project}/${var.environment}/application"
  recovery_window_in_days = var.environment == "production" ? 30 : 0
  tags                    = local.tags
}

resource "aws_secretsmanager_secret_version" "application" {
  secret_id = aws_secretsmanager_secret.application.id
  secret_string = jsonencode({
    POSTGRES_ADMIN_URL          = "postgres://${aws_db_instance.postgres.username}:${random_password.db.result}@${aws_db_instance.postgres.address}:5432/uzip?sslmode=require"
    DB_MASTER_PASSWORD          = random_password.db.result
    APP_DATABASE_PASSWORD       = random_password.app_db.result
    POSTGRES_URL                = "postgres://uzip_app:${random_password.app_db.result}@${aws_db_instance.postgres.address}:5432/uzip?sslmode=require"
    REDIS_URL                   = "rediss://${aws_elasticache_replication_group.redis.primary_endpoint_address}:6379/0"
    KAFKA_BROKERS               = aws_msk_serverless_cluster.kafka.bootstrap_brokers_sasl_iam
    CLICKHOUSE_ADDR             = var.clickhouse_host != "" ? var.clickhouse_host : "clickhouse:9000"
    CLICKHOUSE_HOST             = var.clickhouse_host != "" ? split(":", var.clickhouse_host)[0] : "clickhouse"
    CLICKHOUSE_PORT             = var.clickhouse_host != "" && length(split(":", var.clickhouse_host)) > 1 ? split(":", var.clickhouse_host)[1] : "9000"
    CLICKHOUSE_USER             = var.clickhouse_user
    CLICKHOUSE_PASSWORD         = var.clickhouse_password != "" ? var.clickhouse_password : random_password.clickhouse.result
    MAXMIND_ACCOUNT_ID          = var.maxmind_account_id
    MAXMIND_LICENSE_KEY         = var.maxmind_license_key
    NOTIFICATION_WEBHOOK_URL    = var.notification_webhook_url
    NOTIFICATION_WEBHOOK_TOKEN  = var.notification_webhook_token
    NOTIFICATION_WEBHOOK_FORMAT = var.notification_webhook_format
  })
}

resource "aws_s3_bucket" "clickhouse_backups" {
  bucket_prefix = "${local.name}-clickhouse-backups-"
  tags          = local.tags
}

resource "aws_s3_bucket_versioning" "clickhouse_backups" {
  bucket = aws_s3_bucket.clickhouse_backups.id
  versioning_configuration { status = "Enabled" }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "clickhouse_backups" {
  bucket = aws_s3_bucket.clickhouse_backups.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_public_access_block" "clickhouse_backups" {
  bucket                  = aws_s3_bucket.clickhouse_backups.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_lifecycle_configuration" "clickhouse_backups" {
  bucket = aws_s3_bucket.clickhouse_backups.id
  rule {
    id     = "retention"
    status = "Enabled"
    filter {}
    expiration {
      days = var.environment == "production" ? 90 : 14
    }
  }
}

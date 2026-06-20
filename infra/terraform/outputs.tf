output "cluster_name" { value = module.eks.cluster_name }
output "postgres_endpoint" { value = aws_db_instance.postgres.address }
output "redis_endpoint" { value = aws_elasticache_replication_group.redis.primary_endpoint_address }
output "kafka_arn" { value = aws_msk_serverless_cluster.kafka.arn }
output "cognito_issuer" { value = "https://cognito-idp.${var.region}.amazonaws.com/${aws_cognito_user_pool.main.id}" }
output "cognito_client_id" { value = aws_cognito_user_pool_client.dashboard.id }
output "database_password" {
  value     = random_password.db.result
  sensitive = true
}
output "workload_role_arn" { value = aws_iam_role.workload.arn }
output "rds_identifier" { value = aws_db_instance.postgres.id }
output "db_subnet_group" { value = aws_db_subnet_group.main.name }
output "data_security_group_id" { value = aws_security_group.data.id }
output "application_secret_arn" { value = aws_secretsmanager_secret.application.arn }
output "clickhouse_backup_bucket" { value = aws_s3_bucket.clickhouse_backups.id }
output "certificate_arn" { value = var.domain_name == "" ? null : aws_acm_certificate_validation.main[0].certificate_arn }
output "waf_acl_arn" { value = var.domain_name == "" ? null : aws_wafv2_web_acl.main[0].arn }
output "cognito_domain" { value = "https://${aws_cognito_user_pool_domain.main.domain}.auth.${var.region}.amazoncognito.com" }
output "application_domains" {
  value = {
    dashboard = local.app_domain
    api       = local.api_domain
    redirect  = local.redirect_domain
    analytics = local.analytics_domain
  }
}

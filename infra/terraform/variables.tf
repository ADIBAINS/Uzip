variable "region" {
  type    = string
  default = "us-east-1"
}
variable "environment" {
  type    = string
  default = "staging"
}
variable "project" {
  type    = string
  default = "uzip"
}
variable "db_instance_class" {
  type    = string
  default = "db.t4g.micro"
}
variable "domain_name" {
  description = "Base Route53 domain, for example example.com."
  type        = string
  default     = ""
}
variable "hosted_zone_id" {
  description = "Route53 hosted zone ID for domain_name."
  type        = string
  default     = ""
}
variable "alert_email" {
  type      = string
  default   = ""
  sensitive = true
}
variable "monthly_budget_usd" {
  type    = number
  default = 10
}
variable "allow_expensive_stack" {
  description = "Explicit acknowledgement that this EKS/RDS/ElastiCache/MSK/NAT stack can cost far more than AWS Free Plan credits."
  type        = bool
  default     = false
}
variable "clickhouse_host" {
  description = "Managed production ClickHouse native endpoint."
  type        = string
  default     = ""
}
variable "clickhouse_user" {
  type    = string
  default = "default"
}
variable "clickhouse_password" {
  type      = string
  default   = ""
  sensitive = true
}
variable "maxmind_account_id" {
  type      = string
  default   = ""
  sensitive = true
}
variable "maxmind_license_key" {
  type      = string
  default   = ""
  sensitive = true
}
variable "github_repository" {
  description = "GitHub repository in owner/name form for Actions OIDC."
  type        = string
  default     = ""
}

locals {
  name             = "${var.project}-${var.environment}"
  tags             = { Project = var.project, Environment = var.environment, ManagedBy = "Terraform" }
  app_domain       = var.domain_name == "" ? "" : "app.${var.domain_name}"
  api_domain       = var.domain_name == "" ? "" : "api.${var.domain_name}"
  redirect_domain  = var.domain_name == "" ? "" : "u.${var.domain_name}"
  analytics_domain = var.domain_name == "" ? "" : "analytics.${var.domain_name}"
}

variable "notification_webhook_url" {
  type        = string
  default     = ""
  sensitive   = true
  description = "Optional notification delivery webhook URL."
}
variable "notification_webhook_token" {
  type        = string
  default     = ""
  sensitive   = true
  description = "Optional bearer token for the notification delivery webhook."
}

variable "notification_webhook_format" {
  type        = string
  default     = "webhook"
  description = "Notification delivery protocol, webhook or novu."
  validation {
    condition     = contains(["webhook", "novu"], var.notification_webhook_format)
    error_message = "Notification webhook format must be webhook or novu."
  }
}

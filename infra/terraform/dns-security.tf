resource "aws_cognito_user_pool_domain" "main" {
  domain       = "${local.name}-auth"
  user_pool_id = aws_cognito_user_pool.main.id
}

resource "aws_acm_certificate" "main" {
  count             = var.domain_name == "" ? 0 : 1
  domain_name       = var.domain_name
  validation_method = "DNS"
  subject_alternative_names = [
    "*.${var.domain_name}"
  ]
  lifecycle { create_before_destroy = true }
  tags = local.tags
}

resource "aws_route53_record" "certificate_validation" {
  for_each = var.domain_name == "" ? {} : {
    for option in aws_acm_certificate.main[0].domain_validation_options :
    option.domain_name => {
      name   = option.resource_record_name
      record = option.resource_record_value
      type   = option.resource_record_type
    }
  }
  zone_id = var.hosted_zone_id
  name    = each.value.name
  type    = each.value.type
  records = [each.value.record]
  ttl     = 60
}

resource "aws_acm_certificate_validation" "main" {
  count                   = var.domain_name == "" ? 0 : 1
  certificate_arn         = aws_acm_certificate.main[0].arn
  validation_record_fqdns = [for record in aws_route53_record.certificate_validation : record.fqdn]
}

resource "aws_wafv2_web_acl" "main" {
  count = var.domain_name == "" ? 0 : 1
  name  = local.name
  scope = "REGIONAL"
  default_action {
    allow {}
  }

  rule {
    name     = "AWSManagedCommon"
    priority = 10
    override_action {
      none {}
    }
    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesCommonRuleSet"
        vendor_name = "AWS"
      }
    }
    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${local.name}-common"
      sampled_requests_enabled   = true
    }
  }

  rule {
    name     = "PerIPRateLimit"
    priority = 20
    action {
      block {}
    }
    statement {
      rate_based_statement {
        limit              = 2000
        aggregate_key_type = "IP"
      }
    }
    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${local.name}-rate"
      sampled_requests_enabled   = true
    }
  }

  visibility_config {
    cloudwatch_metrics_enabled = true
    metric_name                = local.name
    sampled_requests_enabled   = true
  }
  tags = local.tags
}

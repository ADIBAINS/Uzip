resource "terraform_data" "cost_acknowledgement" {
  lifecycle {
    precondition {
      condition     = var.allow_expensive_stack
      error_message = "Cost safety lock: this stack is not suitable for a $100 AWS Free Plan. Read docs/aws-100-credit-plan.md. Set allow_expensive_stack=true only if you intentionally accept paid infrastructure."
    }
  }
}

check "production_inputs" {
  assert {
    condition     = var.environment != "production" || var.domain_name != ""
    error_message = "Production requires domain_name."
  }
  assert {
    condition     = var.environment != "production" || var.hosted_zone_id != ""
    error_message = "Production requires hosted_zone_id."
  }
  assert {
    condition     = var.environment != "production" || var.clickhouse_host != ""
    error_message = "Production requires a managed clickhouse_host."
  }
  assert {
    condition     = var.environment != "production" || var.alert_email != ""
    error_message = "Production requires alert_email."
  }
  assert {
    condition     = var.environment != "production" || var.github_repository != ""
    error_message = "Production requires github_repository."
  }
}

# Production readiness and deployment

> Cost warning: this production architecture is intentionally highly available
> and is not suitable for the AWS $100 Free Plan. Terraform is locked by
> default. Read [aws-100-credit-plan.md](aws-100-credit-plan.md) before creating
> AWS resources.

## Required account inputs

The repository now implements the production mechanisms, but applying them requires:

- an AWS account and deployment role;
- a Route53 hosted zone and base domain;
- a managed ClickHouse native endpoint and credentials;
- an alert email;
- a GitHub repository name;
- optional MaxMind account and license credentials.

Provide these through Terraform variables, never committed files.

## Infrastructure order

1. Apply `infra/bootstrap` once to create encrypted remote state and locking.
2. Copy `infra/terraform/backend.tf.example` to `backend.tf`, replacing its outputs.
3. Configure `production.tfvars`.
4. Run `terraform init -migrate-state`, `terraform plan`, and reviewed `terraform apply`.
5. Confirm the SNS email subscription.
6. Configure GitHub Actions variables from Terraform outputs.
7. Push to `main` to publish images and update staging GitOps values.
8. Validate staging and manually promote the immutable SHA to production.

## Required production variables

```hcl
region                = "us-east-1"
environment           = "production"
project               = "uzip"
domain_name           = "example.com"
hosted_zone_id         = "Z123456789"
alert_email           = "oncall@example.com"
monthly_budget_usd     = 10
allow_expensive_stack  = true # only after reviewing the expected paid cost
clickhouse_host        = "example.aws.clickhouse.cloud:9440"
clickhouse_user        = "default"
clickhouse_password    = "..."
github_repository      = "owner/uzip"
maxmind_account_id     = "..."
maxmind_license_key    = "..."
```

Store sensitive variables through a secure CI variable store or an encrypted tfvars mechanism.

## GitHub Actions variables

- `AWS_REGION`
- `AWS_ACCOUNT_ID`
- `AWS_RELEASE_ROLE_ARN`
- `APP_DOMAIN`
- `API_DOMAIN`
- `ANALYTICS_DOMAIN`
- `COGNITO_DOMAIN`
- `COGNITO_CLIENT_ID`
- `RDS_IDENTIFIER`
- `DB_SUBNET_GROUP`
- `DATA_SECURITY_GROUP_ID`
- `APPLICATION_SECRET_ID`

Protect the GitHub `production` environment with required reviewers.

## ClickHouse strategy

Production Helm values disable the single-node in-cluster ClickHouse instance. Use ClickHouse Cloud or another replicated managed deployment with provider-managed availability and backups.

For staging or self-hosted use, the chart provides an S3 backup CronJob. Test restoration with:

```bash
BACKUP_NAME=scheduled-YYYYMMDD-HHMMSS \
CLICKHOUSE_HOST=... \
CLICKHOUSE_PASSWORD=... \
S3_BUCKET=... \
bash scripts/clickhouse-restore.sh
```

Run restoration into an isolated ClickHouse target first. Validate table counts and representative analytics queries before changing production endpoints.

## RDS restore drills

The `RDS Restore Drill` workflow restores the latest recoverable point to a temporary private instance, queries tenant and link tables, and deletes the temporary instance.

It deliberately requires a self-hosted GitHub runner inside the VPC and production environment approval. Run it quarterly and retain the workflow record as evidence.

## DNS, TLS, WAF, and rate limiting

Terraform provisions:

- a wildcard ACM certificate with Route53 DNS validation;
- Cognito's hosted login domain;
- a regional WAF ACL using AWS managed common rules;
- a WAF per-IP rate rule;
- ExternalDNS for application host records;
- the AWS Load Balancer Controller for ALB ingresses.

Application-level Redis limits remain as defense in depth.

## Secrets

Terraform stores application connection material in AWS Secrets Manager. External Secrets Operator copies it into a namespaced Kubernetes Secret using the Uzip IRSA service account.

The Git repository contains no production connection strings. Rotate database and ClickHouse passwords through Terraform or a dedicated rotation workflow, then wait for External Secrets reconciliation and restart affected deployments.

## Database and Kafka changes

Argo CD applies PostgreSQL, ClickHouse, and Kafka topic Jobs before application Deployments:

- PostgreSQL migrations execute with the administrator URL and configure `uzip_app` as a restricted login.
- Services connect only as `uzip_app`, allowing forced RLS to enforce tenant boundaries.
- ClickHouse migrations are idempotent.
- Kafka topic provisioning uses MSK IAM and creates raw, retry, and DLQ topics.

## Acceptance gates

Before promotion:

```bash
go test -race ./...
bash scripts/test-integration.sh
cd web && npm ci && npm run lint && npm run build && npm audit
helm lint deploy/helm/uzip
terraform validate
```

Then run the staging load-test workflow at the intended request rate and verify:

- redirect p95 latency;
- error rate;
- MSK consumer lag;
- DLQ count;
- ClickHouse insertion latency;
- RDS and Redis saturation;
- no cross-tenant access.

# AWS $100 credit plan

## Decision

Do not apply `infra/terraform` on the AWS Free Plan. That stack creates EKS,
EC2 worker nodes, NAT Gateway, RDS, ElastiCache, MSK Serverless, load balancers,
public IPv4 addresses, and storage. EKS control-plane pricing alone is about
$73 for a 730-hour month before worker nodes or any other service.

Terraform therefore has a cost safety lock. Leave:

```hcl
allow_expensive_stack = false
monthly_budget_usd    = 10
```

The budget sends alerts; it does not stop AWS resources automatically.

## Recommended portfolio workflow

1. Keep the complete Kubernetes/Terraform implementation in the repository to
   demonstrate production architecture.
2. Run the full application locally with Docker Compose.
3. Record screenshots or a short demo video for the portfolio.
4. Use AWS only for short, deliberate experiments, then destroy resources the
   same day.
5. Do not upgrade the AWS account to Paid Plan merely to run this project.

The AWS Free Plan provides $100 initially, can provide up to another $100 for
completing eligible activities, lasts up to six months, and does not charge
overages unless the account is upgraded or paid-only services are activated.
Service availability is limited on the Free Plan.

## Account setup now

- Enable MFA on the root user and do not create root access keys.
- Create a normal administrator identity through IAM Identity Center.
- Open Billing and Cost Management and enable billing alerts.
- Create a $10 monthly AWS Budget with email notifications at 50%, 80%, and
  100%.
- Turn on Free Tier usage alerts.
- Use one region only (`us-east-1`) to make cleanup easier.
- Check Cost Explorer and the Free Tier page after every experiment.

## Services to avoid for this credit limit

- EKS: charged per cluster-hour, before worker-node costs.
- NAT Gateway: hourly and data-processing charges.
- MSK and managed ClickHouse: inappropriate for a tiny portfolio budget.
- Always-on RDS, ElastiCache, ALB, and multiple EC2 instances.
- Route 53 domains: domain registration is a separate real cost.

## If an AWS demonstration is required

Use a temporary EC2 instance and Docker Compose only for a scheduled demo:

1. Create the instance immediately before the demo.
2. Use a small encrypted EBS volume and restrict SSH to your IP.
3. Start only the services needed for the demo.
4. Stop or terminate the instance afterward.
5. Delete unused EBS volumes, snapshots, Elastic IPs, load balancers, and NAT
   gateways.

This application includes Kafka and ClickHouse, so a very small instance may
run out of memory. Do not leave a larger instance running continuously.

## Cost safety lock

The full stack can only be planned or applied after explicitly setting:

```hcl
allow_expensive_stack = true
```

That switch is an acknowledgement, not a cost cap. Prefer keeping it `false`
while using Free Plan credits.

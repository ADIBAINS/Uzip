#!/usr/bin/env bash
set -euo pipefail

: "${SOURCE_DB_IDENTIFIER:?Set SOURCE_DB_IDENTIFIER}"
: "${DB_SUBNET_GROUP:?Set DB_SUBNET_GROUP}"
: "${VPC_SECURITY_GROUP_ID:?Set VPC_SECURITY_GROUP_ID}"
: "${APPLICATION_SECRET_ID:?Set APPLICATION_SECRET_ID}"

DRILL_ID="${SOURCE_DB_IDENTIFIER}-restore-drill-$(date +%Y%m%d%H%M%S)"
cleanup() {
  aws rds delete-db-instance --db-instance-identifier "$DRILL_ID" --skip-final-snapshot >/dev/null 2>&1 || true
}
trap cleanup EXIT

aws rds restore-db-instance-to-point-in-time \
  --source-db-instance-identifier "$SOURCE_DB_IDENTIFIER" \
  --target-db-instance-identifier "$DRILL_ID" \
  --use-latest-restorable-time \
  --db-subnet-group-name "$DB_SUBNET_GROUP" \
  --vpc-security-group-ids "$VPC_SECURITY_GROUP_ID" \
  --no-publicly-accessible

aws rds wait db-instance-available --db-instance-identifier "$DRILL_ID"
ENDPOINT="$(aws rds describe-db-instances --db-instance-identifier "$DRILL_ID" --query 'DBInstances[0].Endpoint.Address' --output text)"
PASSWORD="$(aws secretsmanager get-secret-value --secret-id "$APPLICATION_SECRET_ID" --query SecretString --output text | jq -r .DB_MASTER_PASSWORD)"

PGPASSWORD="$PASSWORD" psql "host=$ENDPOINT port=5432 dbname=uzip user=uzip sslmode=require" \
  -v ON_ERROR_STOP=1 \
  -c "SELECT count(*) AS tenant_count FROM tenants" \
  -c "SELECT count(*) AS link_count FROM links"

echo "Restore drill succeeded for $DRILL_ID"

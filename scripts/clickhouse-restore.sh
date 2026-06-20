#!/usr/bin/env bash
set -euo pipefail

: "${BACKUP_NAME:?Set BACKUP_NAME to a remote backup name}"
: "${CLICKHOUSE_HOST:?Set CLICKHOUSE_HOST}"
: "${CLICKHOUSE_PASSWORD:?Set CLICKHOUSE_PASSWORD}"
: "${S3_BUCKET:?Set S3_BUCKET}"

docker run --rm \
  -e CLICKHOUSE_HOST \
  -e CLICKHOUSE_PASSWORD \
  -e CLICKHOUSE_USER="${CLICKHOUSE_USER:-default}" \
  -e REMOTE_STORAGE=s3 \
  -e S3_BUCKET \
  -e S3_PATH="${S3_PATH:-production}" \
  -e AWS_REGION="${AWS_REGION:-us-east-1}" \
  -e AWS_ACCESS_KEY_ID \
  -e AWS_SECRET_ACCESS_KEY \
  -e AWS_SESSION_TOKEN \
  altinity/clickhouse-backup:2.6.39 \
  clickhouse-backup restore_remote "$BACKUP_NAME"

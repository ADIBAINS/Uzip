# Operations runbook

## Service degradation

1. Check `/ready`, Kubernetes events, restart counts, and Grafana request/error panels.
2. Inspect the matching trace in Jaeger using the W3C trace ID from logs.
3. Check PostgreSQL connections, Redis latency, MSK consumer lag, ClickHouse disk, and the `clicks.dlq.v1` topic.
4. Roll back by restoring the previous immutable image digest in Git and allowing Argo CD to reconcile.

Redirects intentionally remain available when ClickHouse or the analytics API is down. Kafka buffers accepted click events. Redis failure increases PostgreSQL load; scale the redirect service only after confirming PostgreSQL has capacity.

## Kafka lag or DLQ growth

- Confirm event-processor replicas are ready and can authenticate through IRSA.
- Check ClickHouse insertion errors and disk pressure.
- Scale consumers only up to the raw topic partition count.
- After fixing the cause, replay DLQ records into `clicks.raw.v1` while preserving `event_id`; ReplacingMergeTree deduplicates during merges.

## Backup and restore

- Enable RDS automated backups and test point-in-time restoration quarterly.
- Snapshot ClickHouse persistent volumes before disruptive upgrades.
- Store Terraform state in encrypted S3 with DynamoDB locking.
- Validate a full staging restore before declaring backups healthy.

## Production promotion

1. CI must pass tests, vulnerability checks, image scans, and dashboard build.
2. Deploy the immutable digest to staging through GitOps.
3. Run API smoke tests and `tests/load/redirect.js`.
4. Approve the production values change and monitor latency, errors, Kafka lag, and DLQ growth for 30 minutes.

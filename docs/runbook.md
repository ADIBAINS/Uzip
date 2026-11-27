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

## Dashboard sessions and notifications

The dashboard calls `POST /api/graphql` on its own origin. The gateway also accepts Cognito bearer tokens for programmatic GraphQL requests when development auth is disabled. Queries include `links`, `link`, `notifications`, `analyticsSummary`, `analyticsTimeseries`, and `analyticsBreakdown`. Mutations include `createLink` and `archiveLink`. See the [GraphQL schema](../web/app/server/graphql.ts).

Set the dashboard's server-side `CONTROL_API_URL`, `ANALYTICS_API_URL`, and `REDIS_URL`. In production, set `NEXT_PUBLIC_DEV_AUTH=false` and configure the Cognito hosted domain, app client ID, registered `/auth/callback` URL, and registered logout URL. Workspace administrators assign the tenant ID and role in Cognito. For local development with a custom host, set `UZIP_DASHBOARD_ORIGIN` to the exact origin used to open the dashboard.

The Notifications page shows successful link creation, declined alias conflicts, milestones at 100 and 1,000 human clicks, and reminders in the 24 hours before an active link expires. Links created after applying migration `004_link_notifications.sql` have creator metadata for these events. The production event processor consumes Kafka in a separate notification group; the Render demo generates milestones from its PostgreSQL click ingestion.

Optional outbound delivery uses `NOTIFICATION_WEBHOOK_URL` (HTTPS) and `NOTIFICATION_WEBHOOK_TOKEN` (bearer authentication). The control service sends a JSON POST with `id`, `tenant_id`, `recipient_id`, `recipient_email` (a verified email or empty), `alias`, `kind`, `created_at`, and applicable `link_id`, `milestone`, or `expires_at`. A delivery provider can route the event to the recipient's configured email, SMS, or push channel. Acknowledge success with a 2xx and deduplicate retried deliveries using the `Idempotency-Key` header. Notifications also appear in the dashboard inbox without a provider.

For direct Novu delivery, set `NOTIFICATION_WEBHOOK_FORMAT=novu` and put the Novu environment key in `NOTIFICATION_WEBHOOK_TOKEN`. Publish the workflows `uzip-link-created`, `uzip-link-creation-failed`, `uzip-link-click-milestone`, and `uzip-link-expiring`. Configure each workflow's email, SMS, or push steps in Novu. Subscribers are keyed by Cognito user ID, and Novu needs a phone number or registered device for SMS or push. The worker sends the notification ID as both the idempotency key and transaction ID. [Novu trigger API](https://docs.novu.co/api-reference/events/trigger-event)

CREATE TABLE IF NOT EXISTS link_notification_targets (
    link_id uuid PRIMARY KEY REFERENCES links(id) ON DELETE CASCADE,
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    recipient_id text NOT NULL,
    recipient_email text NOT NULL DEFAULT '',
    alias text NOT NULL,
    expires_at timestamptz,
    canceled boolean NOT NULL DEFAULT false,
    expiry_notified boolean NOT NULL DEFAULT false,
    click_count integer NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS notification_expiry_idx ON link_notification_targets (expires_at)
    WHERE NOT canceled AND NOT expiry_notified AND expires_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS notification_clicks (
    event_id uuid PRIMARY KEY,
    link_id uuid NOT NULL REFERENCES link_notification_targets(link_id) ON DELETE CASCADE,
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS notification_clicks_link_idx ON notification_clicks (link_id);

CREATE TABLE IF NOT EXISTS link_notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    recipient_id text NOT NULL,
    recipient_email text NOT NULL DEFAULT '',
    link_id uuid REFERENCES links(id) ON DELETE CASCADE,
    alias text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('created', 'creation_failed', 'click_milestone', 'expiring')),
    milestone integer NOT NULL DEFAULT 0,
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    next_attempt timestamptz NOT NULL DEFAULT now(),
    delivery_attempts integer NOT NULL DEFAULT 0,
    delivered_at timestamptz,
    UNIQUE (link_id, kind, milestone)
);
CREATE INDEX IF NOT EXISTS notification_inbox_idx ON link_notifications (tenant_id, recipient_id, created_at DESC);
CREATE INDEX IF NOT EXISTS notification_delivery_idx ON link_notifications (next_attempt) WHERE delivered_at IS NULL;

DO $$
DECLARE notification_table text;
BEGIN
    FOREACH notification_table IN ARRAY ARRAY['link_notification_targets','notification_clicks','link_notifications'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', notification_table);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', notification_table);
        EXECUTE format('DROP POLICY IF EXISTS notification_tenant_isolation ON %I', notification_table);
        EXECUTE format(
            'CREATE POLICY notification_tenant_isolation ON %I USING (
                tenant_id = nullif(current_setting(''app.tenant_id'', true), '''')::uuid
                OR current_setting(''app.notification_worker'', true) = ''true''
            )', notification_table
        );
        EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON %I TO uzip_app', notification_table);
    END LOOP;
END
$$;

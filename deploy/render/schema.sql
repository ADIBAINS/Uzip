GRANT uzip_app TO CURRENT_USER;
GRANT USAGE ON SCHEMA public TO uzip_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO uzip_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO uzip_app;

CREATE TABLE IF NOT EXISTS demo_clicks (
    event_id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    link_id uuid NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    occurred_at timestamptz NOT NULL,
    visitor_id text NOT NULL,
    country text NOT NULL,
    device text NOT NULL,
    browser text NOT NULL,
    os text NOT NULL,
    referrer text NOT NULL
);

CREATE INDEX IF NOT EXISTS demo_clicks_tenant_time_idx ON demo_clicks (tenant_id, occurred_at);
ALTER TABLE demo_clicks ENABLE ROW LEVEL SECURITY;
ALTER TABLE demo_clicks FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS demo_clicks_tenant_isolation ON demo_clicks;
CREATE POLICY demo_clicks_tenant_isolation ON demo_clicks
    USING (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);
GRANT SELECT, INSERT ON demo_clicks TO uzip_app;

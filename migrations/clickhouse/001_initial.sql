CREATE DATABASE IF NOT EXISTS uzip;

CREATE TABLE IF NOT EXISTS uzip.click_events (
    event_id UUID,
    occurred_at DateTime64(3, 'UTC'),
    tenant_id UUID,
    link_id UUID,
    alias LowCardinality(String),
    visitor_id FixedString(32),
    referrer String,
    country LowCardinality(String),
    device LowCardinality(String),
    browser LowCardinality(String),
    os LowCardinality(String),
    user_agent String,
    is_bot UInt8 DEFAULT 0,
    inserted_at DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(inserted_at)
PARTITION BY toYYYYMM(occurred_at)
ORDER BY (tenant_id, link_id, occurred_at, event_id)
TTL toDateTime(occurred_at) + INTERVAL 13 MONTH DELETE;

ALTER TABLE uzip.click_events ADD COLUMN IF NOT EXISTS is_bot UInt8 DEFAULT 0 AFTER user_agent;

package store

import (
	"context"
	"fmt"
	"time"

	"github.com/adilbains/uzip/internal/model"
)

const demoClickFilter = `
	FROM demo_clicks
	WHERE tenant_id = $1 AND occurred_at >= $2 AND occurred_at < $3
	  AND (NULLIF($4, '') IS NULL OR link_id = NULLIF($4, '')::uuid)
`

func (p *Postgres) RecordDemoClick(ctx context.Context, event model.ClickEvent) error {
	if event.IsBot {
		return nil
	}
	tx, err := p.tenantTx(ctx, event.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO demo_clicks
		(event_id, tenant_id, link_id, occurred_at, visitor_id, country, device, browser, os, referrer)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT (event_id) DO NOTHING`,
		event.EventID, event.TenantID, event.LinkID, event.OccurredAt, event.VisitorID,
		event.Country, event.Device, event.Browser, event.OS, event.Referrer)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) Summary(ctx context.Context, tenantID, linkID string, from, to time.Time) (model.AnalyticsSummary, error) {
	var result model.AnalyticsSummary
	tx, err := p.tenantTx(ctx, tenantID)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `SELECT count(*), count(DISTINCT visitor_id)`+demoClickFilter,
		tenantID, from, to, linkID).Scan(&result.TotalClicks, &result.UniqueVisitors)
	return result, err
}

func (p *Postgres) TimeSeries(ctx context.Context, tenantID, linkID, interval string, from, to time.Time) ([]model.TimePoint, error) {
	switch interval {
	case "hour", "week":
	default:
		interval = "day"
	}
	tx, err := p.tenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT date_trunc($5, occurred_at, 'UTC'), count(*), count(DISTINCT visitor_id)`+
		demoClickFilter+` GROUP BY 1 ORDER BY 1`, tenantID, from, to, linkID, interval)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	points := []model.TimePoint{}
	for rows.Next() {
		var point model.TimePoint
		if err := rows.Scan(&point.Timestamp, &point.Clicks, &point.Unique); err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	return points, rows.Err()
}

func (p *Postgres) Breakdown(ctx context.Context, tenantID, linkID, dimension string, from, to time.Time) ([]model.Breakdown, error) {
	switch dimension {
	case "country", "device", "browser", "os", "referrer":
	default:
		return nil, fmt.Errorf("unsupported dimension")
	}
	tx, err := p.tenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	query := fmt.Sprintf(`SELECT coalesce(nullif(%s, ''), 'Unknown'), count(*)`, dimension)
	rows, err := tx.Query(ctx, query+demoClickFilter+` GROUP BY 1 ORDER BY 2 DESC LIMIT 20`, tenantID, from, to, linkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []model.Breakdown{}
	for rows.Next() {
		var result model.Breakdown
		if err := rows.Scan(&result.Name, &result.Clicks); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

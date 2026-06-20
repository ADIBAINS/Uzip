package analytics

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/adilbains/uzip/internal/model"
)

type Store struct{ conn clickhouse.Conn }

func Open(addr, database, user, password string, secure bool) (*Store, error) {
	options := &clickhouse.Options{
		Addr:        []string{addr},
		Auth:        clickhouse.Auth{Database: database, Username: user, Password: password},
		DialTimeout: 5 * time.Second,
	}
	if secure {
		options.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	conn, err := clickhouse.Open(options)
	if err != nil {
		return nil, err
	}
	return &Store{conn: conn}, nil
}

func (s *Store) Ping(ctx context.Context) error { return s.conn.Ping(ctx) }
func (s *Store) Close() error                   { return s.conn.Close() }

func (s *Store) Insert(ctx context.Context, event model.ClickEvent) error {
	return s.conn.Exec(ctx, `INSERT INTO click_events
		(event_id, occurred_at, tenant_id, link_id, alias, visitor_id, referrer, country, device, browser, os, user_agent, is_bot)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.EventID, event.OccurredAt, event.TenantID, event.LinkID, event.Alias, event.VisitorID,
		event.Referrer, event.Country, event.Device, event.Browser, event.OS, event.UserAgent, event.IsBot)
}

func (s *Store) Summary(ctx context.Context, tenantID, linkID string, from, to time.Time) (model.AnalyticsSummary, error) {
	query := `SELECT count(), uniq(visitor_id) FROM click_events WHERE tenant_id=? AND occurred_at>=? AND occurred_at<? AND is_bot=0`
	args := []any{tenantID, from, to}
	if linkID != "" {
		query += " AND link_id=?"
		args = append(args, linkID)
	}
	var result model.AnalyticsSummary
	err := s.conn.QueryRow(ctx, query, args...).Scan(&result.TotalClicks, &result.UniqueVisitors)
	return result, err
}

func (s *Store) TimeSeries(ctx context.Context, tenantID, linkID, interval string, from, to time.Time) ([]model.TimePoint, error) {
	bucket := map[string]string{"hour": "toStartOfHour", "day": "toStartOfDay", "week": "toStartOfWeek"}[interval]
	if bucket == "" {
		bucket = "toStartOfDay"
	}
	query := fmt.Sprintf(`SELECT %s(occurred_at) bucket, count(), uniq(visitor_id)
		FROM click_events WHERE tenant_id=? AND occurred_at>=? AND occurred_at<? AND is_bot=0`, bucket)
	args := []any{tenantID, from, to}
	if linkID != "" {
		query += " AND link_id=?"
		args = append(args, linkID)
	}
	query += " GROUP BY bucket ORDER BY bucket"
	rows, err := s.conn.Query(ctx, query, args...)
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

func (s *Store) Breakdown(ctx context.Context, tenantID, linkID, dimension string, from, to time.Time) ([]model.Breakdown, error) {
	allowed := map[string]string{"country": "country", "device": "device", "browser": "browser", "os": "os", "referrer": "referrer"}
	column, ok := allowed[dimension]
	if !ok {
		return nil, fmt.Errorf("unsupported dimension")
	}
	query := fmt.Sprintf(`SELECT if(empty(%s),'Unknown',%s), count() clicks FROM click_events
		WHERE tenant_id=? AND occurred_at>=? AND occurred_at<? AND is_bot=0`, column, column)
	args := []any{tenantID, from, to}
	if linkID != "" {
		query += " AND link_id=?"
		args = append(args, linkID)
	}
	query += " GROUP BY 1 ORDER BY clicks DESC LIMIT 20"
	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []model.Breakdown{}
	for rows.Next() {
		var item model.Breakdown
		if err := rows.Scan(&item.Name, &item.Clicks); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

package store

import (
	"context"
	"errors"
	"time"

	"github.com/adilbains/uzip/internal/model"
	"github.com/jackc/pgx/v5"
)

const notificationFields = "id, link_id, alias, kind, milestone, expires_at, created_at"

func (p *Postgres) ListNotifications(ctx context.Context, principal model.Principal) ([]model.Notification, error) {
	tx, err := p.tenantTx(ctx, principal.TenantID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT `+notificationFields+`
		FROM link_notifications WHERE tenant_id=$1 AND recipient_id=$2
		ORDER BY created_at DESC, id DESC LIMIT 50`, principal.TenantID, principal.UserID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[model.Notification])
}

func (p *Postgres) RecordCreationFailure(ctx context.Context, principal model.Principal, alias string) error {
	tx, err := p.tenantTx(ctx, principal.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `
		INSERT INTO link_notifications (tenant_id, recipient_id, recipient_email, alias, kind)
		VALUES ($1,$2,$3,$4,'creation_failed')`, principal.TenantID, principal.UserID, principal.VerifiedEmail(), alias)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RecordNotificationClick deduplicates replays before advancing the creator's milestone counter.
func (p *Postgres) RecordNotificationClick(ctx context.Context, event model.ClickEvent) error {
	if event.IsBot {
		return nil
	}
	tx, err := p.tenantTx(ctx, event.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := recordNotificationClick(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func recordNotificationClick(ctx context.Context, tx pgx.Tx, event model.ClickEvent) error {
	var count int
	err := tx.QueryRow(ctx, `
		SELECT click_count FROM link_notification_targets
		WHERE link_id=$1 AND tenant_id=$2 AND NOT canceled AND click_count < 1000 FOR UPDATE`,
		event.LinkID, event.TenantID).Scan(&count)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO notification_clicks (event_id, link_id, tenant_id)
		VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, event.EventID, event.LinkID, event.TenantID)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	count++
	if _, err := tx.Exec(ctx, `UPDATE link_notification_targets SET click_count=$2 WHERE link_id=$1`, event.LinkID, count); err != nil {
		return err
	}
	if count == 100 || count == 1000 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO link_notifications (link_id, tenant_id, recipient_id, recipient_email, alias, kind, milestone)
			SELECT link_id, tenant_id, recipient_id, recipient_email, alias, 'click_milestone', $2
			FROM link_notification_targets WHERE link_id=$1
			ON CONFLICT (link_id, kind, milestone) DO NOTHING`, event.LinkID, count); err != nil {
			return err
		}
	}
	if count == 1000 {
		if _, err := tx.Exec(ctx, `DELETE FROM notification_clicks WHERE link_id=$1`, event.LinkID); err != nil {
			return err
		}
	}
	return nil
}

func (p *Postgres) QueueExpiryNotifications(ctx context.Context) error {
	tx, err := p.notificationTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `
		WITH due AS (
			SELECT link_id FROM link_notification_targets
			WHERE NOT canceled AND NOT expiry_notified AND expires_at > now() AND expires_at <= now() + interval '24 hours'
			ORDER BY expires_at LIMIT 100 FOR UPDATE SKIP LOCKED
		), targets AS (
			UPDATE link_notification_targets SET expiry_notified=true
			WHERE link_id IN (SELECT link_id FROM due)
			RETURNING link_id, tenant_id, recipient_id, recipient_email, alias, expires_at
		)
		INSERT INTO link_notifications (link_id, tenant_id, recipient_id, recipient_email, alias, kind, expires_at)
		SELECT link_id, tenant_id, recipient_id, recipient_email, alias, 'expiring', expires_at FROM targets
		ON CONFLICT (link_id, kind, milestone) DO NOTHING`)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type NotificationDelivery struct {
	model.Notification
	TenantID       string `json:"tenant_id"`
	RecipientID    string `json:"recipient_id"`
	RecipientEmail string `json:"recipient_email"`
	Attempt        int    `json:"-"`
}

func (p *Postgres) ClaimNotification(ctx context.Context) (NotificationDelivery, error) {
	var delivery NotificationDelivery
	tx, err := p.notificationTx(ctx)
	if err != nil {
		return delivery, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `
		UPDATE link_notifications SET next_attempt=now() + interval '1 minute', delivery_attempts=delivery_attempts+1
		WHERE id=(
			SELECT id FROM link_notifications AS pending WHERE delivered_at IS NULL AND next_attempt <= now()
			AND (kind <> 'expiring' OR (expires_at > now() AND EXISTS (
				SELECT 1 FROM link_notification_targets WHERE link_id=pending.link_id AND NOT canceled
			)))
			ORDER BY next_attempt LIMIT 1 FOR UPDATE SKIP LOCKED
		) RETURNING `+notificationFields+`, tenant_id, recipient_id, recipient_email, delivery_attempts`).
		Scan(&delivery.ID, &delivery.LinkID, &delivery.Alias, &delivery.Kind, &delivery.Milestone,
			&delivery.ExpiresAt, &delivery.CreatedAt, &delivery.TenantID, &delivery.RecipientID, &delivery.RecipientEmail, &delivery.Attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return delivery, ErrNotFound
	}
	if err != nil {
		return delivery, err
	}
	return delivery, tx.Commit(ctx)
}

func (p *Postgres) FinishNotification(ctx context.Context, delivery NotificationDelivery, retryAfter time.Duration) error {
	tx, err := p.notificationTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE link_notifications SET
		delivered_at=CASE WHEN $3=0 THEN now() ELSE delivered_at END,
		next_attempt=now() + make_interval(secs => $3::double precision)
		WHERE id=$1 AND delivery_attempts=$2`, delivery.ID, delivery.Attempt, retryAfter.Seconds())
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) notificationTx(ctx context.Context) (pgx.Tx, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.notification_worker', 'true', true)`); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

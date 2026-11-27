package store

import (
	"context"
	"errors"

	"github.com/adilbains/uzip/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Postgres struct{ Pool *pgxpool.Pool }

func OpenPostgres(ctx context.Context, url string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Postgres{Pool: pool}, nil
}

func (p *Postgres) Close()                         { p.Pool.Close() }
func (p *Postgres) Ping(ctx context.Context) error { return p.Pool.Ping(ctx) }

func (p *Postgres) CreateLink(ctx context.Context, principal model.Principal, input model.CreateLink) (model.Link, error) {
	link := model.Link{ID: uuid.NewString(), TenantID: principal.TenantID, Alias: input.Alias, Destination: input.Destination, Redirect: input.Redirect, ExpiresAt: input.ExpiresAt}
	tx, err := p.tenantTx(ctx, principal.TenantID)
	if err != nil {
		return link, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `
		INSERT INTO links (id, tenant_id, alias, destination, redirect_type, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING created_at, updated_at`,
		link.ID, link.TenantID, link.Alias, link.Destination, link.Redirect, link.ExpiresAt,
	).Scan(&link.CreatedAt, &link.UpdatedAt)
	if err != nil {
		return link, err
	}
	if principal.UserID != "" {
		_, err = tx.Exec(ctx, `
			WITH target AS (
				INSERT INTO link_notification_targets (link_id, tenant_id, recipient_id, recipient_email, alias, expires_at)
				VALUES ($1,$2,$3,$4,$5,$6)
				RETURNING link_id, tenant_id, recipient_id, recipient_email, alias
			)
			INSERT INTO link_notifications (link_id, tenant_id, recipient_id, recipient_email, alias, kind)
			SELECT link_id, tenant_id, recipient_id, recipient_email, alias, 'created' FROM target`,
			link.ID, link.TenantID, principal.UserID, principal.VerifiedEmail(), link.Alias, link.ExpiresAt)
		if err != nil {
			return link, err
		}
	}
	return link, tx.Commit(ctx)
}

func (p *Postgres) ListLinks(ctx context.Context, tenantID, search string, limit, offset int) ([]model.Link, error) {
	tx, err := p.tenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
		SELECT id, tenant_id, alias, destination, redirect_type, expires_at, archived_at, created_at, updated_at
		FROM links WHERE tenant_id=$1
		AND ($2='' OR strpos(lower(alias), lower($2)) > 0 OR strpos(lower(destination), lower($2)) > 0)
		ORDER BY created_at DESC, id DESC LIMIT $3 OFFSET $4`, tenantID, search, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	links := []model.Link{}
	for rows.Next() {
		var link model.Link
		if err := rows.Scan(&link.ID, &link.TenantID, &link.Alias, &link.Destination, &link.Redirect, &link.ExpiresAt, &link.ArchivedAt, &link.CreatedAt, &link.UpdatedAt); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return links, tx.Commit(ctx)
}

func (p *Postgres) GetLink(ctx context.Context, tenantID, id string) (model.Link, error) {
	tx, err := p.tenantTx(ctx, tenantID)
	if err != nil {
		return model.Link{}, err
	}
	defer tx.Rollback(ctx)
	link, err := scanLink(tx.QueryRow(ctx, `
		SELECT id, tenant_id, alias, destination, redirect_type, expires_at, archived_at, created_at, updated_at
		FROM links WHERE tenant_id=$1 AND id=$2`, tenantID, id))
	if err == nil {
		err = tx.Commit(ctx)
	}
	return link, err
}

func (p *Postgres) ResolveAlias(ctx context.Context, alias string) (model.Link, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return model.Link{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT set_config('app.public_redirect', 'true', true)`); err != nil {
		return model.Link{}, err
	}
	link, err := scanLink(tx.QueryRow(ctx, `
		SELECT id, tenant_id, alias, destination, redirect_type, expires_at, archived_at, created_at, updated_at
		FROM links WHERE alias=$1`, alias))
	if err == nil {
		err = tx.Commit(ctx)
	}
	return link, err
}

func (p *Postgres) ArchiveLink(ctx context.Context, tenantID, id string) (model.Link, error) {
	tx, err := p.tenantTx(ctx, tenantID)
	if err != nil {
		return model.Link{}, err
	}
	defer tx.Rollback(ctx)
	link, err := scanLink(tx.QueryRow(ctx, `
		UPDATE links SET archived_at=COALESCE(archived_at, now()),
			updated_at=CASE WHEN archived_at IS NULL THEN now() ELSE updated_at END
		WHERE tenant_id=$1 AND id=$2
		RETURNING id, tenant_id, alias, destination, redirect_type, expires_at, archived_at, created_at, updated_at`,
		tenantID, id))
	if err != nil {
		return model.Link{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE link_notification_targets SET canceled=true WHERE link_id=$1 AND NOT canceled`, link.ID); err != nil {
		return link, err
	}
	return link, tx.Commit(ctx)
}

func (p *Postgres) CreateAPIKey(ctx context.Context, tenantID, name, prefix string, hash []byte) (model.APIKey, error) {
	key := model.APIKey{ID: uuid.NewString(), TenantID: tenantID, Name: name, Prefix: prefix}
	tx, err := p.tenantTx(ctx, tenantID)
	if err != nil {
		return key, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `INSERT INTO api_keys (id,tenant_id,name,key_prefix,key_hash) VALUES ($1,$2,$3,$4,$5) RETURNING created_at`,
		key.ID, key.TenantID, key.Name, key.Prefix, hash).Scan(&key.CreatedAt)
	if err == nil {
		err = tx.Commit(ctx)
	}
	return key, err
}

func (p *Postgres) ListAPIKeys(ctx context.Context, tenantID string) ([]model.APIKey, error) {
	tx, err := p.tenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id,tenant_id,name,key_prefix,last_used_at,revoked_at,created_at FROM api_keys WHERE tenant_id=$1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []model.APIKey{}
	for rows.Next() {
		var key model.APIKey
		if err := rows.Scan(&key.ID, &key.TenantID, &key.Name, &key.Prefix, &key.LastUsedAt, &key.RevokedAt, &key.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return keys, tx.Commit(ctx)
}

func (p *Postgres) RevokeAPIKey(ctx context.Context, tenantID, id string) error {
	tx, err := p.tenantTx(ctx, tenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE api_keys SET revoked_at=now() WHERE tenant_id=$1 AND id=$2 AND revoked_at IS NULL`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

type rowScanner interface{ Scan(...any) error }

func scanLink(row rowScanner) (model.Link, error) {
	var link model.Link
	err := row.Scan(&link.ID, &link.TenantID, &link.Alias, &link.Destination, &link.Redirect, &link.ExpiresAt, &link.ArchivedAt, &link.CreatedAt, &link.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return link, ErrNotFound
	}
	return link, err
}

func (p *Postgres) EnsureDevelopmentTenant(ctx context.Context) error {
	_, err := p.Pool.Exec(ctx, `
		INSERT INTO tenants (id,name,slug) VALUES ('00000000-0000-0000-0000-000000000001','Demo Organization','demo')
		ON CONFLICT (id) DO NOTHING;
		INSERT INTO users (id,email) VALUES ('00000000-0000-0000-0000-000000000001','demo@uzip.local')
		ON CONFLICT (id) DO NOTHING;
		INSERT INTO memberships (tenant_id,user_id,role) VALUES ('00000000-0000-0000-0000-000000000001','00000000-0000-0000-0000-000000000001','owner')
		ON CONFLICT DO NOTHING;`)
	return err
}

func (p *Postgres) tenantTx(ctx context.Context, tenantID string) (pgx.Tx, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tenantID); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

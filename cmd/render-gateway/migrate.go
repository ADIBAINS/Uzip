package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
)

func migrate(ctx context.Context, databaseURL, directory string) error {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	defer conn.Close(context.Background())

	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(686635382721321786)`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS render_schema_migrations (version integer PRIMARY KEY)`); err != nil {
		return err
	}
	var applied bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM render_schema_migrations WHERE version = 1)`).Scan(&applied); err != nil {
		return err
	}
	if !applied {
		for _, name := range []string{"001_initial.sql", "schema.sql"} {
			sql, err := os.ReadFile(filepath.Join(directory, name))
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return fmt.Errorf("apply %s: %w", name, err)
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO render_schema_migrations VALUES (1)`); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

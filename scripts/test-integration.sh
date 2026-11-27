#!/usr/bin/env bash
set -euo pipefail

cleanup() { docker compose -f tests/integration/compose.yaml down -v; }
trap cleanup EXIT
docker compose -f tests/integration/compose.yaml up -d --wait
export TEST_DATABASE_URL="postgres://uzip_app:change-me-in-production@localhost:55432/uzip?sslmode=disable"
export TEST_DATABASE_ADMIN_URL="postgres://postgres:postgres@localhost:55432/uzip?sslmode=disable"
export TEST_REDIS_URL="redis://localhost:56379/0"
go test -tags=integration ./internal/store -count=1

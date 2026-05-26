//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

const testDSN = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"

func newPool(t *testing.T, ctx context.Context, dbName string) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(testDSN)
	require.NoError(t, err)
	cfg.ConnConfig.Database = dbName
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	require.NoError(t, pool.Ping(ctx))
	t.Cleanup(pool.Close)
	return pool
}

func createSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	name := "pgsize_" + randSuffix()
	_, err := pool.Exec(ctx, "CREATE SCHEMA "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		pool.Exec(ctx, "DROP SCHEMA "+name+" CASCADE") //nolint:errcheck
	})
	return name
}

func createDatabase(t *testing.T, ctx context.Context, adminPool *pgxpool.Pool) string {
	t.Helper()
	name := "pgsize_" + randSuffix()
	_, err := adminPool.Exec(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		adminPool.Exec(ctx, "DROP DATABASE "+name) //nolint:errcheck
	})
	return name
}

func randSuffix() string {
	b := make([]byte, 4)
	rand.Read(b) //nolint:errcheck
	return hex.EncodeToString(b)
}

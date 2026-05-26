//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashmap-kz/pgsize/internal/pg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSizeArithmetic verifies that the size values returned by ListRelations
// are internally consistent: heap == pg_table_size, and the sum of all parts
// does not exceed pg_total_relation_size.
func TestSizeArithmetic(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t, ctx, "postgres")
	schema := createSchema(t, ctx, pool)

	tbl := schema + "_t"
	_, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE %s.%s (id serial PRIMARY KEY, payload text);
		INSERT INTO %s.%s (payload)
		SELECT repeat('x', 4000) FROM generate_series(1, 200);
	`, schema, tbl, schema, tbl))
	require.NoError(t, err)

	// collect the pg_total_relation_size via raw query for comparison
	var totalBytes int64
	err = pool.QueryRow(ctx, fmt.Sprintf(
		"SELECT pg_total_relation_size('%s.%s'::regclass)::bigint", schema, tbl,
	)).Scan(&totalBytes)
	require.NoError(t, err)

	rels, err := pg.ListRelations(ctx, pool, schema, tbl)
	require.NoError(t, err)
	require.NotEmpty(t, rels)

	var heapBytes uint64
	var sumBytes uint64
	for _, r := range rels {
		sumBytes += r.SizeBytes
		if r.Kind == pg.RelHeap {
			heapBytes = r.SizeBytes
		}
	}

	assert.Greater(t, heapBytes, uint64(0), "heap row must have non-zero size")
	// heap excludes toast so that toast row is not double-counted;
	// all parts must add up exactly to pg_total_relation_size
	assert.Equal(t, uint64(totalBytes), sumBytes, "sum of parts must equal pg_total_relation_size")
}

// TestIndexGrouping verifies that ListTables correctly groups indexes per table:
// a table with no indexes produces an empty Indexes slice, and a table with
// three indexes produces exactly three entries.
func TestIndexGrouping(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t, ctx, "postgres")
	schema := createSchema(t, ctx, pool)

	t.Run("no indexes", func(t *testing.T) {
		tbl := "no_idx"
		_, err := pool.Exec(ctx, fmt.Sprintf(
			"CREATE TABLE %s.%s (id int, val text)", schema, tbl,
		))
		require.NoError(t, err)

		tbls, err := pg.ListTables(ctx, pool, schema)
		require.NoError(t, err)

		var found *pg.Table
		for i := range tbls {
			if tbls[i].Name == tbl {
				found = &tbls[i]
				break
			}
		}
		require.NotNil(t, found, "table not found in ListTables result")
		assert.Empty(t, found.Indexes, "table with no indexes must have empty Indexes slice")
	})

	t.Run("three indexes", func(t *testing.T) {
		tbl := "three_idx"
		_, err := pool.Exec(ctx, fmt.Sprintf(`
			CREATE TABLE %s.%s (id serial PRIMARY KEY, a int, b text);
			CREATE INDEX ON %s.%s (a);
			CREATE INDEX ON %s.%s (b);
		`, schema, tbl, schema, tbl, schema, tbl))
		require.NoError(t, err)

		tbls, err := pg.ListTables(ctx, pool, schema)
		require.NoError(t, err)

		var found *pg.Table
		for i := range tbls {
			if tbls[i].Name == tbl {
				found = &tbls[i]
				break
			}
		}
		require.NotNil(t, found, "table not found in ListTables result")
		assert.Len(t, found.Indexes, 3, "table must have exactly 3 indexes")
	})
}

// TestIndexAccessMethods verifies that ListRelations maps each PostgreSQL AM
// name to the correct RelKind constant.
func TestIndexAccessMethods(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t, ctx, "postgres")
	schema := createSchema(t, ctx, pool)

	tbl := "am_test"
	_, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE %s.%s (
			id    serial PRIMARY KEY,
			col_a int,
			col_b tsvector,
			col_c int,
			col_d int[]
		);
		CREATE INDEX idx_btree ON %s.%s USING btree (col_a);
		CREATE INDEX idx_gin   ON %s.%s USING gin   (col_b);
		CREATE INDEX idx_hash  ON %s.%s USING hash  (col_c);
		CREATE INDEX idx_brin  ON %s.%s USING brin  (col_a);
	`, schema, tbl,
		schema, tbl,
		schema, tbl,
		schema, tbl,
		schema, tbl,
	))
	require.NoError(t, err)

	rels, err := pg.ListRelations(ctx, pool, schema, tbl)
	require.NoError(t, err)

	// build a map by name for easy lookup
	byName := make(map[string]pg.Relation)
	for _, r := range rels {
		byName[r.Name] = r
	}

	cases := []struct {
		indexName string
		wantKind  pg.RelKind
	}{
		{"idx_btree", pg.RelBtree},
		{"idx_gin", pg.RelGin},
		{"idx_hash", pg.RelHash},
		{"idx_brin", pg.RelBrin},
	}

	for _, tc := range cases {
		t.Run(tc.indexName, func(t *testing.T) {
			r, ok := byName[tc.indexName]
			require.True(t, ok, "index %q not found in ListRelations result", tc.indexName)
			assert.Equal(t, tc.wantKind, r.Kind,
				"index %q: expected kind %v, got %v", tc.indexName, tc.wantKind, r.Kind)
		})
	}
}

// TestConnectionRouting verifies that withDatabasePool-style per-DB pools route
// to the correct database. A schema created in a temporary database must not be
// visible when querying the default postgres database.
func TestConnectionRouting(t *testing.T) {
	ctx := context.Background()
	adminPool := newPool(t, ctx, "postgres")

	// create an isolated temp database
	tmpDB := createDatabase(t, ctx, adminPool)
	tmpPool := newPool(t, ctx, tmpDB)

	// create a marker schema in the temp database
	markerSchema := createSchema(t, ctx, tmpPool)

	// it must be visible in the temp DB
	tmpSchemas, err := pg.ListSchemas(ctx, tmpPool)
	require.NoError(t, err)
	var foundInTmp bool
	for _, s := range tmpSchemas {
		if s.Name == markerSchema {
			foundInTmp = true
			break
		}
	}
	assert.True(t, foundInTmp, "marker schema must be visible in the temp database")

	// it must NOT be visible in the default postgres database
	defaultSchemas, err := pg.ListSchemas(ctx, adminPool)
	require.NoError(t, err)
	var foundInDefault bool
	for _, s := range defaultSchemas {
		if s.Name == markerSchema {
			foundInDefault = true
			break
		}
	}
	assert.False(t, foundInDefault, "marker schema must not be visible in the postgres database")
}

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

func TestListSchemas_includesCreatedSchema(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t, ctx, "postgres")
	schema := createSchema(t, ctx, pool)

	schemas, err := pg.ListSchemas(ctx, pool)
	require.NoError(t, err)

	var found bool
	for _, s := range schemas {
		if s.Name == schema {
			found = true
			break
		}
	}
	assert.True(t, found, "created schema must appear in ListSchemas")
}

func TestListSchemas_excludesSystemSchemas(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t, ctx, "postgres")

	schemas, err := pg.ListSchemas(ctx, pool)
	require.NoError(t, err)

	system := map[string]bool{
		"pg_catalog": true, "information_schema": true, "pg_toast": true,
	}
	for _, s := range schemas {
		assert.False(t, system[s.Name], "system schema %q must not appear", s.Name)
	}
}

func TestListTables_indexGrouping(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t, ctx, "postgres")
	schema := createSchema(t, ctx, pool)

	exec(t, ctx, pool, fmt.Sprintf(`CREATE TABLE %s.no_idx (id int)`, schema))
	exec(t, ctx, pool, fmt.Sprintf(`
		CREATE TABLE %s.three_idx (id serial PRIMARY KEY, a int, b text);
		CREATE INDEX ON %s.three_idx (a);
		CREATE INDEX ON %s.three_idx (b);
	`, schema, schema, schema))

	tbls, err := pg.ListTables(ctx, pool, schema)
	require.NoError(t, err)

	byName := make(map[string]pg.Table)
	for _, tbl := range tbls {
		byName[tbl.Name] = tbl
	}

	assert.Empty(t, byName["no_idx"].Indexes, "table without indexes must have empty Indexes")
	assert.Len(t, byName["three_idx"].Indexes, 3, "table must have exactly 3 indexes")
}

func TestListTopTables_crossSchema(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t, ctx, "postgres")
	s1 := createSchema(t, ctx, pool)
	s2 := createSchema(t, ctx, pool)

	exec(t, ctx, pool, fmt.Sprintf(`
		CREATE TABLE %s.big (id serial, v text);
		INSERT INTO %s.big (v) SELECT repeat('x', 100) FROM generate_series(1, 500);
	`, s1, s1))
	exec(t, ctx, pool, fmt.Sprintf(`
		CREATE TABLE %s.small (id serial);
		INSERT INTO %s.small SELECT generate_series(1, 5);
	`, s2, s2))

	tops, err := pg.ListTopTables(ctx, pool, 50)
	require.NoError(t, err)

	schemas := make(map[string]bool)
	for _, tbl := range tops {
		schemas[tbl.Schema] = true
	}

	assert.True(t, schemas[s1], "schema %s must appear in top tables", s1)
	assert.True(t, schemas[s2], "schema %s must appear in top tables", s2)
}

func TestListRelations_alwaysHasHeap(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t, ctx, "postgres")
	schema := createSchema(t, ctx, pool)

	exec(t, ctx, pool, fmt.Sprintf(`CREATE TABLE %s.t (id int)`, schema))

	rels, err := pg.ListRelations(ctx, pool, schema, "t")
	require.NoError(t, err)

	var hasHeap bool
	for _, r := range rels {
		if r.Kind == pg.RelHeap {
			hasHeap = true
		}
	}
	assert.True(t, hasHeap, "relations must contain a HEAP row")
}

func TestListRelations_indexKinds(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t, ctx, "postgres")
	schema := createSchema(t, ctx, pool)

	exec(t, ctx, pool, fmt.Sprintf(`
		CREATE TABLE %s.t (id serial, a int, b tsvector, c int);
		CREATE INDEX idx_btree ON %s.t USING btree (a);
		CREATE INDEX idx_gin   ON %s.t USING gin   (b);
		CREATE INDEX idx_hash  ON %s.t USING hash  (c);
		CREATE INDEX idx_brin  ON %s.t USING brin  (a);
	`, schema, schema, schema, schema, schema))

	rels, err := pg.ListRelations(ctx, pool, schema, "t")
	require.NoError(t, err)

	byName := make(map[string]pg.Relation)
	for _, r := range rels {
		byName[r.Name] = r
	}

	cases := []struct {
		name string
		kind pg.RelKind
	}{
		{"idx_btree", pg.RelBtree},
		{"idx_gin", pg.RelGin},
		{"idx_hash", pg.RelHash},
		{"idx_brin", pg.RelBrin},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, ok := byName[tc.name]
			require.True(t, ok, "%s not found in relations", tc.name)
			assert.Equal(t, tc.kind, r.Kind)
		})
	}
}

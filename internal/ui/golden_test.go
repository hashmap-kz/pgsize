package ui

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashmap-kz/pgsize/internal/pg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "overwrite golden files with current output")

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o600))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "golden file missing — run: go test -run TestGolden ./internal/ui/... -update")
	assert.Equal(t, string(want), got)
}

// fixture data shared across all golden tests — deterministic, representative

var goldenDatabases = []pg.Database{
	{Name: "production", SizeBytes: 45 * 1 << 30},
	{Name: "staging", SizeBytes: 3*1<<30 + 512*1<<20},
	{Name: "analytics", SizeBytes: 820 * 1 << 20},
}

var goldenSchemas = []pg.Schema{
	{Name: "public", SizeBytes: 12 * 1 << 30, RowCount: 8_500_000, TableCount: 42, IndexCount: 67},
	{Name: "analytics", SizeBytes: 3 * 1 << 30, RowCount: 1_200_000, TableCount: 8, IndexCount: 12},
	{Name: "audit", SizeBytes: 450 * 1 << 20, RowCount: 400_000, TableCount: 3, IndexCount: 5},
}

var goldenTables = []pg.Table{
	{Schema: "public", Name: "events", TotalBytes: 8 * 1 << 30, RowCount: 5_000_000, BloatPct: 12.5, Indexes: make([]pg.Index, 4)},
	{Schema: "public", Name: "users", TotalBytes: 2 * 1 << 30, RowCount: 800_000, BloatPct: 0, Indexes: make([]pg.Index, 2)},
	{Schema: "public", Name: "sessions", TotalBytes: 600 * 1 << 20, RowCount: 200_000, BloatPct: 38.2, Indexes: make([]pg.Index, 1)},
}

var goldenRelations = []pg.Relation{
	{Name: "table data", Kind: pg.RelHeap, SizeBytes: 7 * 1 << 30},
	{Name: "toast", Kind: pg.RelToast, SizeBytes: 512 * 1 << 20},
	{Name: "events_user_idx", Kind: pg.RelBtree, SizeBytes: 800 * 1 << 20},
	{Name: "events_ts_idx", Kind: pg.RelBtree, SizeBytes: 400 * 1 << 20},
}

var goldenTopTables = []pg.Table{
	{Schema: "public", Name: "events", TotalBytes: 8 * 1 << 30, RowCount: 5_000_000, BloatPct: 12.5},
	{Schema: "analytics", Name: "page_views", TotalBytes: 3 * 1 << 30, RowCount: 2_000_000, BloatPct: 0},
	{Schema: "public", Name: "sessions", TotalBytes: 600 * 1 << 20, RowCount: 200_000, BloatPct: 38.2},
}

func TestGolden_databases(t *testing.T) {
	DisableStyles()
	m := newTestModel(viewDatabases, goldenDatabases, nil, nil, nil)
	checkGolden(t, "golden_databases.txt", renderOf(m))
}

func TestGolden_schemas(t *testing.T) {
	DisableStyles()
	m := newTestModel(viewSchemas, goldenDatabases, goldenSchemas, nil, nil)
	//nolint:goconst
	m.curDB = "production"
	checkGolden(t, "golden_schemas.txt", renderOf(m))
}

func TestGolden_tables(t *testing.T) {
	DisableStyles()
	m := newTestModel(viewTables, goldenDatabases, goldenSchemas, goldenTables, nil)
	m.curDB = "production"
	m.curSchema = "public"
	checkGolden(t, "golden_tables.txt", renderOf(m))
}

func TestGolden_relations(t *testing.T) {
	DisableStyles()
	m := newTestModel(viewRelations, goldenDatabases, goldenSchemas, goldenTables, goldenRelations)
	m.curDB = "production"
	m.curSchema = "public"
	m.curTable = "events"
	checkGolden(t, "golden_relations.txt", renderOf(m))
}

func TestGolden_topTables(t *testing.T) {
	DisableStyles()
	m := newTestModel(viewTopTables, goldenDatabases, nil, nil, nil)
	m.topTbls = goldenTopTables
	m.curDB = "production"
	checkGolden(t, "golden_top_tables.txt", renderOf(m))
}

func TestGolden_loading(t *testing.T) {
	DisableStyles()
	m := newTestModel(viewDatabases, nil, nil, nil, nil)
	m.loading = true
	checkGolden(t, "golden_loading.txt", renderOf(m))
}

func TestGolden_error(t *testing.T) {
	DisableStyles()
	m := newTestModel(viewDatabases, nil, nil, nil, nil)
	m.err = errors.New("connection refused: dial tcp 127.0.0.1:5432")
	checkGolden(t, "golden_error.txt", renderOf(m))
}

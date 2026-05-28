package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/hashmap-kz/pgsize/internal/pg"
	"github.com/stretchr/testify/assert"
)

// renderOf sets terminal dimensions and returns the full rendered output.
func renderOf(m model) string {
	m.width, m.height = 120, 40
	return m.View()
}

// lineWith returns the first line in out that contains substr, or "".
func lineWith(out, substr string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, substr) {
			return line
		}
	}
	return ""
}

func TestBreadcrumb(t *testing.T) {
	cases := []struct {
		parts []string
		want  string
	}{
		{[]string{"a", "b", "c"}, "a › b › c"},
		{[]string{"only"}, "only"},
		{[]string{}, ""},
	}
	for _, tc := range cases {
		if got := breadcrumb(tc.parts...); got != tc.want {
			t.Errorf("breadcrumb(%v) = %q, want %q", tc.parts, got, tc.want)
		}
	}
}

// --- render (View) tests ---

func TestRenderDatabases_rowContent(t *testing.T) {
	DisableStyles()
	dbs := []pg.Database{{Name: "mydb", SizeBytes: 1 << 30}} // 1 GiB
	m := newTestModel(viewDatabases, dbs, nil, nil, nil)

	out := renderOf(m)

	assert.Contains(t, out, "mydb", "database name must appear in output")
	assert.Contains(t, out, "1.0 GB", "humanized size must appear in output")
}

func TestRenderDatabases_cursorMarker(t *testing.T) {
	DisableStyles()
	dbs := []pg.Database{{Name: "first"}, {Name: "second"}, {Name: "third"}}
	m := newTestModel(viewDatabases, dbs, nil, nil, nil)
	m.cursor = 1

	out := renderOf(m)

	assert.Contains(t, lineWith(out, "second"), ">", "cursor must mark the selected row")
	assert.NotContains(t, lineWith(out, "first"), ">", "cursor must not mark other rows")
	assert.NotContains(t, lineWith(out, "third"), ">", "cursor must not mark other rows")
}

func TestRenderDatabases_sortIndicator(t *testing.T) {
	DisableStyles()
	dbs := []pg.Database{{Name: "a", SizeBytes: 100}}
	m := newTestModel(viewDatabases, dbs, nil, nil, nil)

	m.sort = sortSize
	assert.Contains(t, renderOf(m), "SIZE *", "active size sort must mark SIZE column")

	m.sort = sortName
	assert.Contains(t, renderOf(m), "DATABASE *", "active name sort must mark DATABASE column")
}

func TestRenderFooter_databasesHasTopHint(t *testing.T) {
	DisableStyles()
	m := newTestModel(viewDatabases, nil, nil, nil, nil)

	assert.Contains(t, renderOf(m), "[T] top", "databases footer must show top-tables hint")
}

func TestRenderFooter_relationsHasNoSortOrDrillHint(t *testing.T) {
	DisableStyles()
	m := newTestModel(viewRelations, nil, nil, nil, nil)

	out := renderOf(m)

	assert.NotContains(t, out, "[s] sort", "relations footer must not show sort hint (sort is a no-op there)")
	assert.NotContains(t, out, "[enter]", "relations footer must not show drill hint (nothing to drill into)")
}

func TestRenderLoading_bodyAndHints(t *testing.T) {
	DisableStyles()
	m := newTestModel(viewDatabases, nil, nil, nil, nil)
	m.loading = true

	out := renderOf(m)

	assert.Contains(t, out, "loading...", "loading state must show loading message")
	assert.NotContains(t, out, "[hjkl/arrows]", "action hints must be hidden while loading")
}

func TestRenderError_showsMessageAndDismissHint(t *testing.T) {
	DisableStyles()
	m := newTestModel(viewDatabases, nil, nil, nil, nil)
	m.err = errors.New("connection refused")

	out := renderOf(m)

	assert.Contains(t, out, "connection refused", "error message must appear in output")
	assert.Contains(t, out, "backspace", "dismiss hint must appear in error state")
}

// --- pageWindow tests ---

func TestModelPageWindow(t *testing.T) {
	dbs := make([]pg.Database, 20)
	for i := range dbs {
		dbs[i] = pg.Database{Name: "db"}
	}
	m := newTestModel(viewDatabases, dbs, nil, nil, nil)
	m.height = 10 // maxRows = 10 - 6 = 4

	// cursor at 0: window [0, 4)
	m.cursor = 0
	start, end := m.pageWindow(20)
	if start != 0 || end != 4 {
		t.Errorf("pageWindow cursor=0 = (%d, %d), want (0, 4)", start, end)
	}

	// cursor at 3 (last in first page): window [0, 4)
	m.cursor = 3
	start, end = m.pageWindow(20)
	if start != 0 || end != 4 {
		t.Errorf("pageWindow cursor=3 = (%d, %d), want (0, 4)", start, end)
	}

	// cursor at 4 (just beyond first page): start=1, end=5
	m.cursor = 4
	start, end = m.pageWindow(20)
	if start != 1 || end != 5 {
		t.Errorf("pageWindow cursor=4 = (%d, %d), want (1, 5)", start, end)
	}

	// cursor near end: end clamped to n
	m.cursor = 19
	_, end = m.pageWindow(20)
	if end != 20 {
		t.Errorf("pageWindow cursor=19 end = %d, want 20 (clamped)", end)
	}
}

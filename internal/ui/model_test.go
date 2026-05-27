package ui

import (
	"testing"

	"github.com/hashmap-kz/pgsize/internal/pg"
)

const testZZZ = "zzz"

func TestTrunc(t *testing.T) {
	cases := []struct {
		s    string
		n    int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 4, "hel~"},
		{"hello", 3, "he~"},
		{"hello", 2, "h~"},
		{"hello", 1, "~"},
		{"hello", 0, ""},
		{"hello", -1, ""},
		{"", 5, ""},
		{"héllo", 4, "hél~"},
		{"a", 1, "a"},
		{"ab", 1, "~"},
	}
	for _, tc := range cases {
		if got := trunc(tc.s, tc.n); got != tc.want {
			t.Errorf("trunc(%q, %d) = %q, want %q", tc.s, tc.n, got, tc.want)
		}
	}
}

func TestCursor(t *testing.T) {
	if got := cursor(3, 3); got != ">" {
		t.Errorf("cursor(3,3) = %q, want %q", got, ">")
	}
	if got := cursor(2, 3); got != " " {
		t.Errorf("cursor(2,3) = %q, want %q", got, " ")
	}
	if got := cursor(0, 0); got != ">" {
		t.Errorf("cursor(0,0) = %q, want %q", got, ">")
	}
}

func TestHumanize(t *testing.T) {
	const KB = 1024
	const MB = 1024 * KB
	const GB = 1024 * MB

	cases := []struct {
		b    uint64
		want string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{1023, "1023 B"},
		{uint64(KB), "1.0 KB"},
		{uint64(KB + KB/2), "1.5 KB"},
		{uint64(2 * KB), "2.0 KB"},
		{uint64(MB), "1.0 MB"},
		{uint64(GB), "1.0 GB"},
	}
	for _, tc := range cases {
		if got := humanize(tc.b); got != tc.want {
			t.Errorf("humanize(%d) = %q, want %q", tc.b, got, tc.want)
		}
	}
}

func TestHumanizeCount(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{-1, "0"},
		{1, "~1"},
		{999, "~999"},
		{1_000, "~1.0K"},
		{1_500, "~1.5K"},
		{999_999, "~1000.0K"},
		{1_000_000, "~1.0M"},
		{12_500_000, "~12.5M"},
		{1_000_000_000, "~1.0B"},
		{5_250_000_000, "~5.2B"},
	}
	for _, tc := range cases {
		if got := humanizeCount(tc.n); got != tc.want {
			t.Errorf("humanizeCount(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestBloatBar(t *testing.T) {
	cases := []struct {
		pct      float64
		bloatPct float64
		width    int
		want     string
	}{
		{100, 0, 4, "[####]"},   // no bloat: identical to bar()
		{100, 50, 4, "[##!!]"},  // 50% bloat: half live, half dead
		{100, 100, 4, "[!!!!]"}, // 100% bloat: all dead
		{50, 0, 4, "[##  ]"},    // no bloat, half full
		{50, 50, 4, "[#!  ]"},   // half full, half of that is bloat
		{0, 0, 4, "[    ]"},     // empty table
		{0, 75, 4, "[    ]"},    // no size, bloat irrelevant
		{100, 0, 0, "[]"},       // zero width
		{150, 50, 4, "[##!!]"},  // pct clamped to width
	}
	for _, tc := range cases {
		if got := bloatBar(tc.pct, tc.bloatPct, tc.width); got != tc.want {
			t.Errorf(
				"bloatBar(%v, %v, %d) = %q, want %q",
				tc.pct,
				tc.bloatPct,
				tc.width,
				got,
				tc.want,
			)
		}
	}
}

func TestBar(t *testing.T) {
	cases := []struct {
		pct   float64
		width int
		want  string
	}{
		{0, 4, "[    ]"},
		{100, 4, "[####]"},
		{50, 4, "[##  ]"},
		{25, 4, "[#   ]"},
		{75, 4, "[### ]"},
		{0, 0, "[]"},
		{150, 4, "[####]"}, // clamped to width
		{-10, 4, "[    ]"}, // clamped to 0
	}
	for _, tc := range cases {
		if got := bar(tc.pct, tc.width); got != tc.want {
			t.Errorf("bar(%v, %d) = %q, want %q", tc.pct, tc.width, got, tc.want)
		}
	}
}

func TestCacheKeys(t *testing.T) {
	if got := tableCacheKey("db1", "public"); got != "db1\x00public" {
		t.Errorf("tableCacheKey = %q", got)
	}
	if got := relationCacheKey("db1", "public", "users"); got != "db1\x00public\x00users" {
		t.Errorf("relationCacheKey = %q", got)
	}
	// keys must differ when components differ
	if tableCacheKey("a", "b") == tableCacheKey("a\x00b", "") {
		t.Error("tableCacheKey: collision between ('a','b') and ('a\\x00b','')")
	}
	if relationCacheKey("a", "b", "c") == relationCacheKey("a\x00b", "c", "") {
		t.Error("relationCacheKey: collision")
	}
}

func newTestModel(
	view viewKind,
	dbs []pg.Database,
	schs []pg.Schema,
	tbls []pg.Table,
	rels []pg.Relation,
) model {
	return model{
		view: view,
		dbs:  dbs,
		schs: schs,
		tbls: tbls,
		rels: rels,
		clusters: []clusterState{{
			schCache: make(map[string][]pg.Schema),
			tblCache: make(map[string][]pg.Table),
			relCache: make(map[string][]pg.Relation),
		}},
	}
}

func TestModelRowCount(t *testing.T) {
	dbs := []pg.Database{{Name: "a"}, {Name: "b"}}
	schs := []pg.Schema{{Name: "s1"}}
	tbls := []pg.Table{{Name: "t1"}, {Name: "t2"}, {Name: "t3"}}
	rels := []pg.Relation{{Name: "r1"}}

	cases := []struct {
		view viewKind
		want int
	}{
		{viewDatabases, 2},
		{viewSchemas, 1},
		{viewTables, 3},
		{viewRelations, 1},
	}
	for _, tc := range cases {
		m := newTestModel(tc.view, dbs, schs, tbls, rels)
		if got := m.rowCount(); got != tc.want {
			t.Errorf("rowCount() view=%d = %d, want %d", tc.view, got, tc.want)
		}
	}
}

func TestModelMoveBy(t *testing.T) {
	dbs := []pg.Database{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	m := newTestModel(viewDatabases, dbs, nil, nil, nil)
	m.cursor = 0

	m.moveBy(1)
	if m.cursor != 1 {
		t.Errorf("after moveBy(1) cursor = %d, want 1", m.cursor)
	}

	m.moveBy(1)
	if m.cursor != 2 {
		t.Errorf("after moveBy(1) cursor = %d, want 2", m.cursor)
	}

	// clamp at end
	m.moveBy(1)
	if m.cursor != 2 {
		t.Errorf("moveBy(1) at last = %d, want 2 (clamped)", m.cursor)
	}

	// move backward
	m.moveBy(-1)
	if m.cursor != 1 {
		t.Errorf("after moveBy(-1) cursor = %d, want 1", m.cursor)
	}

	// clamp at start
	m.cursor = 0
	m.moveBy(-1)
	if m.cursor != 0 {
		t.Errorf("moveBy(-1) at first = %d, want 0 (clamped)", m.cursor)
	}

	// empty list: stays at 0
	m2 := newTestModel(viewDatabases, nil, nil, nil, nil)
	m2.moveBy(1)
	if m2.cursor != 0 {
		t.Errorf("moveBy on empty list = %d, want 0", m2.cursor)
	}
}

func TestModelCursorPos(t *testing.T) {
	dbs := []pg.Database{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	m := newTestModel(viewDatabases, dbs, nil, nil, nil)

	m.cursor = 0
	pos, total := m.cursorPos()
	if pos != 1 || total != 3 {
		t.Errorf("cursorPos = (%d, %d), want (1, 3)", pos, total)
	}

	m.cursor = 2
	pos, total = m.cursorPos()
	if pos != 3 || total != 3 {
		t.Errorf("cursorPos = (%d, %d), want (3, 3)", pos, total)
	}

	// empty list: pos=0
	m2 := newTestModel(viewDatabases, nil, nil, nil, nil)
	pos, total = m2.cursorPos()
	if pos != 0 || total != 0 {
		t.Errorf("cursorPos empty = (%d, %d), want (0, 0)", pos, total)
	}
}

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

func TestModelApplySort(t *testing.T) {
	t.Run("databases by size", func(t *testing.T) {
		dbs := []pg.Database{
			{Name: "a", SizeBytes: 100},
			{Name: "b", SizeBytes: 300},
			{Name: "c", SizeBytes: 200},
		}
		m := newTestModel(viewDatabases, dbs, nil, nil, nil)
		m.sort = sortSize
		m.applySort()
		if m.dbs[0].Name != "b" || m.dbs[1].Name != "c" || m.dbs[2].Name != "a" {
			t.Errorf("sort by size desc failed: got %v", m.dbs)
		}
	})

	t.Run("databases by name", func(t *testing.T) {
		dbs := []pg.Database{{Name: "c"}, {Name: "a"}, {Name: "b"}}
		m := newTestModel(viewDatabases, dbs, nil, nil, nil)
		m.sort = sortName
		m.applySort()
		if m.dbs[0].Name != "a" || m.dbs[1].Name != "b" || m.dbs[2].Name != "c" {
			t.Errorf("sort by name asc failed: got %v", m.dbs)
		}
	})

	t.Run("schemas by size", func(t *testing.T) {
		schs := []pg.Schema{{Name: "x", SizeBytes: 50}, {Name: "y", SizeBytes: 200}}
		m := newTestModel(viewSchemas, nil, schs, nil, nil)
		m.sort = sortSize
		m.applySort()
		if m.schs[0].Name != "y" {
			t.Errorf("sort schemas by size failed: got %v", m.schs)
		}
	})

	t.Run("schemas by name", func(t *testing.T) {
		schs := []pg.Schema{{Name: "z"}, {Name: "a"}}
		m := newTestModel(viewSchemas, nil, schs, nil, nil)
		m.sort = sortName
		m.applySort()
		if m.schs[0].Name != "a" {
			t.Errorf("sort schemas by name failed: got %v", m.schs)
		}
	})

	t.Run("tables by size", func(t *testing.T) {
		tbls := []pg.Table{{Name: "t1", TotalBytes: 10}, {Name: "t2", TotalBytes: 500}}
		m := newTestModel(viewTables, nil, nil, tbls, nil)
		m.sort = sortSize
		m.applySort()
		if m.tbls[0].Name != "t2" {
			t.Errorf("sort tables by size failed: got %v", m.tbls)
		}
	})

	t.Run("tables by name", func(t *testing.T) {
		tbls := []pg.Table{{Name: testZZZ}, {Name: "aaa"}}
		m := newTestModel(viewTables, nil, nil, tbls, nil)
		m.sort = sortName
		m.applySort()
		if m.tbls[0].Name != "aaa" {
			t.Errorf("sort tables by name failed: got %v", m.tbls)
		}
	})
}

func TestModelDrillOut(t *testing.T) {
	t.Run("empty stack is a no-op", func(t *testing.T) {
		m := newTestModel(viewDatabases, nil, nil, nil, nil)
		m.cursor = 5
		m.drillOut()
		if m.cursor != 5 || m.view != viewDatabases {
			t.Error("drillOut on empty stack must not change state")
		}
	})

	t.Run("restores previous frame", func(t *testing.T) {
		dbs := []pg.Database{{Name: "a"}, {Name: "b"}, {Name: "c"}}
		m := newTestModel(viewSchemas, dbs, nil, nil, nil)
		m.curDB = "mydb"
		m.cursor = 2
		m.stack = []frame{{
			view:   viewDatabases,
			cursor: 2,
			curDB:  "other",
			curSch: "s",
			curTbl: "t",
		}}
		m.drillOut()

		if m.view != viewDatabases {
			t.Errorf("view = %d, want viewDatabases", m.view)
		}
		if m.cursor != 2 {
			t.Errorf("cursor = %d, want 2", m.cursor)
		}
		if m.curDB != "other" {
			t.Errorf("curDB = %q, want 'other'", m.curDB)
		}
		if m.curSchema != "s" {
			t.Errorf("curSchema = %q, want 's'", m.curSchema)
		}
		if m.curTable != "t" {
			t.Errorf("curTable = %q, want 't'", m.curTable)
		}
		if len(m.stack) != 0 {
			t.Errorf("stack must be empty after drillOut, got len=%d", len(m.stack))
		}
	})
}

func TestModelAcceptLoad(t *testing.T) {
	m := newTestModel(viewDatabases, nil, nil, nil, nil)

	// loadID == 0 is never accepted
	if m.acceptLoad(0) {
		t.Error("acceptLoad(0) must return false")
	}

	// non-matching ID
	if m.acceptLoad(1) {
		t.Error("acceptLoad(1) with m.loadID=0 must return false")
	}

	// advance load ID and accept
	id := m.nextLoadID()
	if id != 1 {
		t.Errorf("nextLoadID() = %d, want 1", id)
	}
	if !m.acceptLoad(1) {
		t.Error("acceptLoad(1) with m.loadID=1 must return true")
	}
	if m.acceptLoad(2) {
		t.Error("acceptLoad(2) with m.loadID=1 must return false")
	}

	// second increment
	id2 := m.nextLoadID()
	if id2 != 2 {
		t.Errorf("nextLoadID() = %d, want 2", id2)
	}
	if m.acceptLoad(1) {
		t.Error("stale loadID=1 must no longer be accepted")
	}
	if !m.acceptLoad(2) {
		t.Error("acceptLoad(2) with m.loadID=2 must return true")
	}
}

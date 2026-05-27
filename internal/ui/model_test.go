package ui

import (
	"testing"

	"github.com/hashmap-kz/pgsize/internal/pg"
)

const testZZZ = "zzz"

// newTestModel is a shared test helper used across multiple test files.
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

func TestInitialModel(t *testing.T) {
	t.Run("single cluster starts at viewDatabases", func(t *testing.T) {
		m, ok := InitialModel([]Cluster{{Name: "local"}}).(*model)
		if !ok {
			t.Fatal("InitialModel did not return *model")
		}
		if m.view != viewDatabases {
			t.Errorf("single cluster: view = %d, want viewDatabases", m.view)
		}
		if m.sort != sortSize {
			t.Errorf("default sort = %d, want sortSize", m.sort)
		}
		if len(m.clusters) != 1 {
			t.Errorf("clusters len = %d, want 1", len(m.clusters))
		}
	})

	t.Run("multiple clusters start at viewClusters", func(t *testing.T) {
		m, ok := InitialModel([]Cluster{{Name: "dev"}, {Name: "prod"}}).(*model)
		if !ok {
			t.Fatal("InitialModel did not return *model")
		}
		if m.view != viewClusters {
			t.Errorf("multi cluster: view = %d, want viewClusters", m.view)
		}
		if len(m.clusters) != 2 {
			t.Errorf("clusters len = %d, want 2", len(m.clusters))
		}
	})
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

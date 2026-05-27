package ui

import (
	"testing"

	"github.com/hashmap-kz/pgsize/internal/pg"
)

const (
	testDBName     = "mydb"
	testSchemaName = "public"
)

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

func TestModelDrillInCacheHit(t *testing.T) {
	t.Run("clusters to databases", func(t *testing.T) {
		dbs := []pg.Database{{Name: "db1"}}
		m := model{
			view: viewClusters,
			clusters: []clusterState{
				{
					Cluster:  Cluster{Name: "c1"},
					schCache: make(map[string][]pg.Schema),
					tblCache: make(map[string][]pg.Table),
					relCache: make(map[string][]pg.Relation),
				},
				{
					Cluster:  Cluster{Name: "c2"},
					dbCache:  dbs,
					schCache: make(map[string][]pg.Schema),
					tblCache: make(map[string][]pg.Table),
					relCache: make(map[string][]pg.Relation),
				},
			},
			cursor: 1,
		}
		cmd := m.drillIn()
		if cmd != nil {
			t.Error("cache hit should return nil cmd")
		}
		if m.view != viewDatabases {
			t.Errorf("view = %d, want viewDatabases", m.view)
		}
		if m.curCluster != 1 {
			t.Errorf("curCluster = %d, want 1", m.curCluster)
		}
		if len(m.dbs) != 1 || m.dbs[0].Name != "db1" {
			t.Errorf("dbs = %v, want [{db1}]", m.dbs)
		}
		if len(m.stack) != 1 {
			t.Error("stack should have 1 frame after drillIn")
		}
	})

	t.Run("databases to schemas", func(t *testing.T) {
		dbs := []pg.Database{{Name: testDBName}}
		schs := []pg.Schema{{Name: testSchemaName}}
		m := newTestModel(viewDatabases, dbs, nil, nil, nil)
		m.clusters[0].schCache[testDBName] = schs

		cmd := m.drillIn()
		if cmd != nil {
			t.Error("cache hit should return nil cmd")
		}
		if m.view != viewSchemas {
			t.Errorf("view = %d, want viewSchemas", m.view)
		}
		if m.curDB != testDBName {
			t.Errorf("curDB = %q, want %q", m.curDB, testDBName)
		}
		if len(m.schs) != 1 || m.schs[0].Name != testSchemaName {
			t.Errorf("schs = %v, want [{%s}]", m.schs, testSchemaName)
		}
	})

	t.Run("schemas to tables", func(t *testing.T) {
		schs := []pg.Schema{{Name: testSchemaName}}
		tbls := []pg.Table{{Name: "users"}}
		m := newTestModel(viewSchemas, nil, schs, nil, nil)
		m.curDB = testDBName
		m.clusters[0].tblCache[tableCacheKey(testDBName, testSchemaName)] = tbls

		cmd := m.drillIn()
		if cmd != nil {
			t.Error("cache hit should return nil cmd")
		}
		if m.view != viewTables {
			t.Errorf("view = %d, want viewTables", m.view)
		}
		if m.curSchema != testSchemaName {
			t.Errorf("curSchema = %q, want %q", m.curSchema, testSchemaName)
		}
		if len(m.tbls) != 1 || m.tbls[0].Name != "users" {
			t.Errorf("tbls = %v, want [{users}]", m.tbls)
		}
	})

	t.Run("tables to relations", func(t *testing.T) {
		tbls := []pg.Table{{Name: "users"}}
		rels := []pg.Relation{{Name: "users_pkey"}}
		m := newTestModel(viewTables, nil, nil, tbls, nil)
		m.curDB = testDBName
		m.curSchema = testSchemaName
		m.clusters[0].relCache[relationCacheKey(testDBName, testSchemaName, "users")] = rels

		cmd := m.drillIn()
		if cmd != nil {
			t.Error("cache hit should return nil cmd")
		}
		if m.view != viewRelations {
			t.Errorf("view = %d, want viewRelations", m.view)
		}
		if m.curTable != "users" {
			t.Errorf("curTable = %q, want 'users'", m.curTable)
		}
		if len(m.rels) != 1 || m.rels[0].Name != "users_pkey" {
			t.Errorf("rels = %v, want [{users_pkey}]", m.rels)
		}
	})

	t.Run("relations: deepest level is a no-op", func(t *testing.T) {
		rels := []pg.Relation{{Name: "r1"}}
		m := newTestModel(viewRelations, nil, nil, nil, rels)
		cmd := m.drillIn()
		if cmd != nil {
			t.Error("drillIn at deepest level should return nil")
		}
		if m.view != viewRelations {
			t.Error("view should not change at deepest level")
		}
	})

	t.Run("empty list: no-op", func(t *testing.T) {
		m := newTestModel(viewDatabases, nil, nil, nil, nil)
		cmd := m.drillIn()
		if cmd != nil {
			t.Error("drillIn with empty list should return nil")
		}
		if m.view != viewDatabases {
			t.Error("view should not change when list is empty")
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
		m.curDB = testDBName
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

func TestModelInvalidateDB(t *testing.T) {
	const db1, db2 = "db1", "db2"
	m := newTestModel(viewSchemas, nil, nil, nil, nil)
	c := &m.clusters[0]
	c.schCache[db1] = []pg.Schema{{Name: testSchemaName}}
	c.tblCache[tableCacheKey(db1, testSchemaName)] = []pg.Table{{Name: "t1"}}
	c.relCache[relationCacheKey(db1, testSchemaName, "t1")] = []pg.Relation{}
	c.tblCache[tableCacheKey(db2, testSchemaName)] = []pg.Table{{Name: "t2"}} // unrelated

	m.invalidateDB(db1)

	if _, ok := c.schCache[db1]; ok {
		t.Error("schCache for db1 should be deleted")
	}
	if _, ok := c.tblCache[tableCacheKey(db1, testSchemaName)]; ok {
		t.Error("tblCache for db1.public should be deleted")
	}
	if _, ok := c.relCache[relationCacheKey(db1, testSchemaName, "t1")]; ok {
		t.Error("relCache for db1.public.t1 should be deleted")
	}
	if _, ok := c.tblCache[tableCacheKey(db2, testSchemaName)]; !ok {
		t.Error("tblCache for db2.public should remain (unrelated db)")
	}
}

func TestModelInvalidateSchema(t *testing.T) {
	const db1 = "db1"
	m := newTestModel(viewTables, nil, nil, nil, nil)
	c := &m.clusters[0]
	c.tblCache[tableCacheKey(db1, testSchemaName)] = []pg.Table{{Name: "t1"}}
	c.relCache[relationCacheKey(db1, testSchemaName, "t1")] = []pg.Relation{}
	c.relCache[relationCacheKey(db1, "other", "t2")] = []pg.Relation{} // unrelated schema

	m.invalidateSchema(db1, testSchemaName)

	if _, ok := c.tblCache[tableCacheKey(db1, testSchemaName)]; ok {
		t.Error("tblCache for db1.public should be deleted")
	}
	if _, ok := c.relCache[relationCacheKey(db1, testSchemaName, "t1")]; ok {
		t.Error("relCache for db1.public.t1 should be deleted")
	}
	if _, ok := c.relCache[relationCacheKey(db1, "other", "t2")]; !ok {
		t.Error("relCache for db1.other.t2 should remain (unrelated schema)")
	}
}

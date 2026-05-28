package ui

import (
	"context"
	"strings"

	"github.com/hashmap-kz/pgsize/internal/pg"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jackc/pgx/v5/pgxpool"
)

const topTablesLimit = 25

func (m *model) drillInTopTables() tea.Cmd {
	if m.rowCount() == 0 {
		return nil
	}
	dbName := m.dbs[m.cursor].Name
	f := frame{view: m.view, cursor: m.cursor, curDB: m.curDB}
	m.stack = append(m.stack, f)
	m.curDB = dbName
	m.curSchema = ""
	m.curTable = ""
	c := &m.clusters[m.curCluster]
	if tbls, ok := c.topTblCache[dbName]; ok {
		m.topTbls = tbls
		m.view = viewTopTables
		m.cursor = 0
		return nil
	}
	m.loading = true
	loadID := m.nextLoadID()
	dsn, ci := c.DSN, m.curCluster
	return func() tea.Msg {
		items, err := withDatabasePool(
			context.Background(), dsn, dbName,
			func(ctx context.Context, pool *pgxpool.Pool) ([]pg.Table, error) {
				return pg.ListTopTables(ctx, pool, topTablesLimit)
			},
		)
		return loadedTopTables{loadID: loadID, clusterIdx: ci, db: dbName, items: items, err: err}
	}
}

func (m *model) drillIn() tea.Cmd {
	if m.rowCount() == 0 {
		return nil
	}
	f := frame{
		view: m.view, cursor: m.cursor,
		curDB: m.curDB, curSch: m.curSchema, curTbl: m.curTable,
	}
	switch m.view {
	case viewClusters:
		ci := m.cursor
		m.stack = append(m.stack, f)
		m.curCluster = ci
		m.curDB = ""
		m.curSchema = ""
		m.curTable = ""
		if m.clusters[ci].dbCache != nil {
			m.dbs = m.clusters[ci].dbCache
			m.view = viewDatabases
			m.cursor = 0
			return nil
		}
		m.loading = true
		loadID := m.nextLoadID()
		pool := m.clusters[ci].Pool
		return func() tea.Msg {
			items, err := pg.ListDatabases(context.Background(), pool)
			return loadedDatabases{loadID: loadID, clusterIdx: ci, items: items, err: err}
		}
	case viewDatabases:
		dbName := m.dbs[m.cursor].Name
		m.stack = append(m.stack, f)
		m.curDB = dbName
		m.curSchema = ""
		m.curTable = ""
		c := &m.clusters[m.curCluster]
		if schs, ok := c.schCache[dbName]; ok {
			m.schs = schs
			m.view = viewSchemas
			m.cursor = 0
			return nil
		}
		m.loading = true
		loadID := m.nextLoadID()
		dsn, ci := c.DSN, m.curCluster
		return func() tea.Msg {
			items, err := withDatabasePool(context.Background(), dsn, dbName, pg.ListSchemas)
			return loadedSchemas{loadID: loadID, clusterIdx: ci, db: dbName, items: items, err: err}
		}
	case viewSchemas:
		schemaName := m.schs[m.cursor].Name
		m.stack = append(m.stack, f)
		m.curSchema = schemaName
		m.curTable = ""
		c := &m.clusters[m.curCluster]
		cacheKey := tableCacheKey(m.curDB, schemaName)
		if tbls, ok := c.tblCache[cacheKey]; ok {
			m.tbls = tbls
			m.view = viewTables
			m.cursor = 0
			return nil
		}
		m.loading = true
		loadID := m.nextLoadID()
		dsn, dbName, ci := c.DSN, m.curDB, m.curCluster
		return func() tea.Msg {
			items, err := withDatabasePool(
				context.Background(), dsn, dbName,
				func(ctx context.Context, pool *pgxpool.Pool) ([]pg.Table, error) {
					return pg.ListTables(ctx, pool, schemaName)
				},
			)
			return loadedTables{
				loadID:     loadID,
				clusterIdx: ci,
				db:         dbName,
				schema:     schemaName,
				items:      items,
				err:        err,
			}
		}
	case viewTables:
		tableName := m.tbls[m.cursor].Name
		m.stack = append(m.stack, f)
		m.curTable = tableName
		c := &m.clusters[m.curCluster]
		cacheKey := relationCacheKey(m.curDB, m.curSchema, tableName)
		if rels, ok := c.relCache[cacheKey]; ok {
			m.rels = rels
			m.view = viewRelations
			m.cursor = 0
			return nil
		}
		m.loading = true
		loadID := m.nextLoadID()
		dsn, dbName, schemaName, ci := c.DSN, m.curDB, m.curSchema, m.curCluster
		return func() tea.Msg {
			items, err := withDatabasePool(
				context.Background(), dsn, dbName,
				func(ctx context.Context, pool *pgxpool.Pool) ([]pg.Relation, error) {
					return pg.ListRelations(ctx, pool, schemaName, tableName)
				},
			)
			return loadedRelations{
				loadID: loadID, clusterIdx: ci,
				db: dbName, schema: schemaName, table: tableName,
				items: items, err: err,
			}
		}
	case viewTopTables:
		t := m.topTbls[m.cursor]
		m.stack = append(m.stack, f)
		m.curSchema = t.Schema
		m.curTable = t.Name
		c := &m.clusters[m.curCluster]
		cacheKey := relationCacheKey(m.curDB, t.Schema, t.Name)
		if rels, ok := c.relCache[cacheKey]; ok {
			m.rels = rels
			m.view = viewRelations
			m.cursor = 0
			return nil
		}
		m.loading = true
		loadID := m.nextLoadID()
		dsn, dbName, schemaName, tableName, ci := c.DSN, m.curDB, t.Schema, t.Name, m.curCluster
		return func() tea.Msg {
			items, err := withDatabasePool(
				context.Background(), dsn, dbName,
				func(ctx context.Context, pool *pgxpool.Pool) ([]pg.Relation, error) {
					return pg.ListRelations(ctx, pool, schemaName, tableName)
				},
			)
			return loadedRelations{
				loadID: loadID, clusterIdx: ci,
				db: dbName, schema: schemaName, table: tableName,
				items: items, err: err,
			}
		}
	case viewRelations: // deepest level - nothing to drill into
	}
	return nil
}

func (m *model) drillOut() {
	if len(m.stack) == 0 {
		return
	}
	f := m.stack[len(m.stack)-1]
	m.stack = m.stack[:len(m.stack)-1]
	m.view = f.view
	m.cursor = f.cursor
	m.curDB = f.curDB
	m.curSchema = f.curSch
	m.curTable = f.curTbl
}

func (m *model) reload() tea.Cmd {
	if m.loading {
		return nil
	}
	m.loading = true
	loadID := m.nextLoadID()
	c := &m.clusters[m.curCluster]
	ci := m.curCluster
	switch m.view {
	case viewDatabases:
		c.dbCache = nil
		pool := c.Pool
		return func() tea.Msg {
			items, err := pg.ListDatabases(context.Background(), pool)
			return loadedDatabases{loadID: loadID, clusterIdx: ci, items: items, err: err}
		}
	case viewSchemas:
		dbName := m.curDB
		m.invalidateDB(dbName)
		dsn := c.DSN
		return func() tea.Msg {
			items, err := withDatabasePool(context.Background(), dsn, dbName, pg.ListSchemas)
			return loadedSchemas{loadID: loadID, clusterIdx: ci, db: dbName, items: items, err: err}
		}
	case viewTables:
		dbName, schemaName := m.curDB, m.curSchema
		m.invalidateSchema(dbName, schemaName)
		dsn := c.DSN
		return func() tea.Msg {
			items, err := withDatabasePool(
				context.Background(), dsn, dbName,
				func(ctx context.Context, pool *pgxpool.Pool) ([]pg.Table, error) {
					return pg.ListTables(ctx, pool, schemaName)
				},
			)
			return loadedTables{
				loadID:     loadID,
				clusterIdx: ci,
				db:         dbName,
				schema:     schemaName,
				items:      items,
				err:        err,
			}
		}
	case viewRelations:
		dbName, schemaName, tableName := m.curDB, m.curSchema, m.curTable
		delete(c.relCache, relationCacheKey(dbName, schemaName, tableName))
		dsn := c.DSN
		return func() tea.Msg {
			items, err := withDatabasePool(
				context.Background(), dsn, dbName,
				func(ctx context.Context, pool *pgxpool.Pool) ([]pg.Relation, error) {
					return pg.ListRelations(ctx, pool, schemaName, tableName)
				},
			)
			return loadedRelations{
				loadID: loadID, clusterIdx: ci,
				db: dbName, schema: schemaName, table: tableName,
				items: items, err: err,
			}
		}
	case viewTopTables:
		dbName := m.curDB
		delete(c.topTblCache, dbName)
		dsn := c.DSN
		return func() tea.Msg {
			items, err := withDatabasePool(
				context.Background(), dsn, dbName,
				func(ctx context.Context, pool *pgxpool.Pool) ([]pg.Table, error) {
					return pg.ListTopTables(ctx, pool, topTablesLimit)
				},
			)
			return loadedTopTables{loadID: loadID, clusterIdx: ci, db: dbName, items: items, err: err}
		}
	case viewClusters: // clusters are static connections - nothing to reload
	}
	return nil
}

func withDatabasePool[T any](
	ctx context.Context,
	dsn, dbName string,
	fn func(context.Context, *pgxpool.Pool) ([]T, error),
) ([]T, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.ConnConfig.Database = dbName
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer pool.Close()
	return fn(ctx, pool)
}

func tableCacheKey(db, schema string) string {
	return db + nilByte + schema
}

func relationCacheKey(db, schema, table string) string {
	return db + nilByte + schema + nilByte + table
}

func (m *model) invalidateDB(db string) {
	c := &m.clusters[m.curCluster]
	delete(c.schCache, db)
	delete(c.topTblCache, db)
	prefix := db + nilByte
	for k := range c.tblCache {
		if strings.HasPrefix(k, prefix) {
			delete(c.tblCache, k)
		}
	}
	for k := range c.relCache {
		if strings.HasPrefix(k, prefix) {
			delete(c.relCache, k)
		}
	}
}

func (m *model) invalidateSchema(db, schema string) {
	c := &m.clusters[m.curCluster]
	delete(c.tblCache, tableCacheKey(db, schema))
	prefix := db + nilByte + schema + nilByte
	for k := range c.relCache {
		if strings.HasPrefix(k, prefix) {
			delete(c.relCache, k)
		}
	}
}

func (m *model) initLoad() tea.Cmd {
	if len(m.clusters) == 1 {
		m.loading = true
		loadID := m.nextLoadID()
		pool := m.clusters[0].Pool
		return func() tea.Msg {
			items, err := pg.ListDatabases(context.Background(), pool)
			return loadedDatabases{loadID: loadID, clusterIdx: 0, items: items, err: err}
		}
	}
	return nil
}

func (m *model) acceptLoad(loadID uint64) bool {
	return loadID != 0 && loadID == m.loadID
}

func (m *model) nextLoadID() uint64 {
	m.loadID++
	return m.loadID
}

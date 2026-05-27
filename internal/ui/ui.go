package ui

import (
	"context"

	"github.com/hashmap-kz/pgsize/internal/pg"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jackc/pgx/v5/pgxpool"
)

type viewKind int

const (
	viewClusters viewKind = iota
	viewDatabases
	viewSchemas
	viewTables
	viewRelations
)

const (
	keyBackspace = "backspace"
	colSize      = "SIZE"
	nilByte      = "\x00"
)

type sortMode int

const (
	sortSize sortMode = iota
	sortName
)

// Cluster holds the connection details for one PostgreSQL cluster.
type Cluster struct {
	Name string
	DSN  string
	Pool *pgxpool.Pool
}

type clusterState struct {
	Cluster
	dbCache  []pg.Database
	schCache map[string][]pg.Schema
	tblCache map[string][]pg.Table
	relCache map[string][]pg.Relation
}

type model struct {
	clusters   []clusterState
	curCluster int

	view   viewKind
	dbs    []pg.Database
	schs   []pg.Schema
	tbls   []pg.Table
	rels   []pg.Relation
	stack  []frame
	cursor int
	sort   sortMode

	curDB     string
	curSchema string
	curTable  string

	loading bool
	loadID  uint64

	width  int
	height int
	err    error
}

type frame struct {
	view   viewKind
	cursor int
	curDB  string
	curSch string
	curTbl string
}

// messages

type loadedDatabases struct {
	loadID     uint64
	clusterIdx int
	items      []pg.Database
	err        error
}
type loadedSchemas struct {
	loadID     uint64
	clusterIdx int
	db         string
	items      []pg.Schema
	err        error
}
type loadedTables struct {
	loadID     uint64
	clusterIdx int
	db         string
	schema     string
	items      []pg.Table
	err        error
}
type loadedRelations struct {
	loadID     uint64
	clusterIdx int
	db         string
	schema     string
	table      string
	items      []pg.Relation
	err        error
}

func InitialModel(clusters []Cluster) tea.Model {
	cs := make([]clusterState, len(clusters))
	for i, c := range clusters {
		cs[i] = clusterState{
			Cluster:  c,
			schCache: make(map[string][]pg.Schema),
			tblCache: make(map[string][]pg.Table),
			relCache: make(map[string][]pg.Relation),
		}
	}
	view := viewClusters
	if len(cs) == 1 {
		view = viewDatabases
	}
	return &model{clusters: cs, view: view, sort: sortSize}
}

func (m *model) Init() tea.Cmd {
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

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.updateKey(msg)

	case loadedDatabases:
		if !m.acceptLoad(msg.loadID) || msg.clusterIdx != m.curCluster {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.dbs = msg.items
		m.clusters[msg.clusterIdx].dbCache = msg.items
		m.view = viewDatabases
		m.cursor = 0
		return m, nil

	case loadedSchemas:
		if !m.acceptLoad(msg.loadID) || msg.clusterIdx != m.curCluster || msg.db != m.curDB {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.schs = msg.items
		m.clusters[msg.clusterIdx].schCache[msg.db] = msg.items
		m.view = viewSchemas
		m.cursor = 0
		return m, nil

	case loadedTables:
		if !m.acceptLoad(msg.loadID) || msg.clusterIdx != m.curCluster ||
			msg.db != m.curDB || msg.schema != m.curSchema {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.tbls = msg.items
		m.clusters[msg.clusterIdx].tblCache[tableCacheKey(msg.db, msg.schema)] = msg.items
		m.view = viewTables
		m.cursor = 0
		return m, nil

	case loadedRelations:
		if !m.acceptLoad(msg.loadID) || msg.clusterIdx != m.curCluster ||
			msg.db != m.curDB || msg.schema != m.curSchema || msg.table != m.curTable {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.rels = msg.items
		m.clusters[msg.clusterIdx].relCache[relationCacheKey(msg.db, msg.schema, msg.table)] = msg.items
		m.view = viewRelations
		m.cursor = 0
		return m, nil
	}
	return m, nil
}

func (m *model) acceptLoad(loadID uint64) bool {
	return loadID != 0 && loadID == m.loadID
}

func (m *model) nextLoadID() uint64 {
	m.loadID++
	return m.loadID
}

func (m *model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "q" || msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.loading {
		return m, nil
	}
	if m.err != nil {
		switch msg.String() {
		case keyBackspace, "h", "left":
			m.err = nil
		}
		return m, nil
	}
	switch msg.String() {
	case "j", "down":
		m.moveBy(1)
	case "k", "up":
		m.moveBy(-1)
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		n := m.rowCount()
		if n > 0 {
			m.cursor = n - 1
		}
	case "enter", "l", "right":
		return m, m.drillIn()
	case keyBackspace, "h", "left":
		m.drillOut()
	case "s":
		if m.sort == sortSize {
			m.sort = sortName
		} else {
			m.sort = sortSize
		}
		m.applySort()
		m.cursor = 0
	case "r":
		return m, m.reload()
	}
	return m, nil
}

func (m *model) rowCount() int {
	switch m.view {
	case viewClusters:
		return len(m.clusters)
	case viewDatabases:
		return len(m.dbs)
	case viewSchemas:
		return len(m.schs)
	case viewTables:
		return len(m.tbls)
	case viewRelations:
		return len(m.rels)
	}
	return 0
}

func (m *model) moveBy(delta int) {
	n := m.rowCount()
	if n == 0 {
		m.cursor = 0
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
}

func (m *model) cursorPos() (pos, total int) {
	total = m.rowCount()
	if total > 0 {
		pos = m.cursor + 1
	}
	return pos, total
}

func (m *model) applySort() {
	bySize := m.sort == sortSize
	switch m.view {
	case viewDatabases:
		sortBySizeOrName(
			m.dbs,
			func(d pg.Database) uint64 { return d.SizeBytes },
			func(d pg.Database) string { return d.Name },
			bySize,
		)
	case viewSchemas:
		sortBySizeOrName(
			m.schs,
			func(s pg.Schema) uint64 { return s.SizeBytes },
			func(s pg.Schema) string { return s.Name },
			bySize,
		)
	case viewTables:
		sortBySizeOrName(
			m.tbls,
			func(t pg.Table) uint64 { return t.TotalBytes },
			func(t pg.Table) string { return t.Name },
			bySize,
		)
	case viewClusters: // clusters are ordered by the user's --dsn arguments
	case viewRelations: // relations within a table have no meaningful size-based order
	}
}

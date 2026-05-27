package ui

import (
	"github.com/hashmap-kz/pgsize/internal/pg"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) handleMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
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

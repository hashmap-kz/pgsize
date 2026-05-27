package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hashmap-kz/pgsize/internal/pg"
	"github.com/hashmap-kz/pgsize/internal/x/fmtx"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func sortBySizeOrName[T any](s []T, size func(T) uint64, name func(T) string, bySize bool) {
	if bySize {
		sort.Slice(s, func(i, j int) bool { return size(s[i]) > size(s[j]) })
	} else {
		sort.Slice(s, func(i, j int) bool { return name(s[i]) < name(s[j]) })
	}
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

// rendering

var (
	headerStyle = lipgloss.NewStyle().Bold(true)
	cursorStyle = lipgloss.NewStyle().Bold(true)
	dimStyle    = lipgloss.NewStyle().Faint(true)
	bloatStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

func DisableStyles() {
	headerStyle = lipgloss.NewStyle()
	cursorStyle = lipgloss.NewStyle()
	dimStyle = lipgloss.NewStyle()
	bloatStyle = lipgloss.NewStyle()
}

func (m *model) View() string {
	if m.err != nil {
		return fmt.Sprintf("error: %v\n\n [backspace] dismiss  [q] quit", m.err)
	}

	body := m.renderBody()
	if m.height > 5 {
		if need := m.height - 5 - strings.Count(body, "\n"); need > 0 {
			body += strings.Repeat("\n", need)
		}
	}

	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", m.width))
	b.WriteString("\n")
	b.WriteString(body)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", m.width))
	b.WriteString("\n")
	b.WriteString(m.renderFooter())
	return b.String()
}

func (m *model) curDBSize() uint64 {
	for _, d := range m.dbs {
		if d.Name == m.curDB {
			return d.SizeBytes
		}
	}
	return 0
}

func (m *model) curSchemaSize() uint64 {
	for _, s := range m.schs {
		if s.Name == m.curSchema {
			return s.SizeBytes
		}
	}
	return 0
}

func (m *model) curTableSize() uint64 {
	for _, t := range m.tbls {
		if t.Name == m.curTable {
			return t.TotalBytes
		}
	}
	return 0
}

func breadcrumb(parts ...string) string {
	return strings.Join(parts, " › ")
}

func (m *model) clusterPrefix() string {
	if len(m.clusters) > 1 {
		return fmt.Sprintf("C=%s", m.clusters[m.curCluster].Name)
	}
	return ""
}

func (m *model) renderHeader() string {
	var left, right string
	multi := len(m.clusters) > 1
	switch m.view {
	case viewClusters:
		left = " pgsize"
		right = fmt.Sprintf("%d clusters", len(m.clusters))
	case viewDatabases:
		var total uint64
		for _, d := range m.dbs {
			total += d.SizeBytes
		}
		left = " pgsize"
		if multi {
			left += "  " + m.clusterPrefix()
		}
		right = "total: " + humanize(total)
	case viewSchemas:
		var total uint64
		for _, s := range m.schs {
			total += s.SizeBytes
		}
		dbPart := fmt.Sprintf("D=%s (%s)", m.curDB, humanize(m.curDBSize()))
		parts := []string{dbPart}
		if multi {
			parts = append([]string{m.clusterPrefix()}, parts...)
		}
		left = " pgsize  " + breadcrumb(parts...)
		right = "db: " + humanize(total)
	case viewTables:
		var total uint64
		for _, t := range m.tbls {
			total += t.TotalBytes
		}
		dbPart := fmt.Sprintf("D=%s (%s)", m.curDB, humanize(m.curDBSize()))
		schPart := fmt.Sprintf("S=%s (%s)", m.curSchema, humanize(m.curSchemaSize()))
		parts := []string{dbPart, schPart}
		if multi {
			parts = append([]string{m.clusterPrefix()}, parts...)
		}
		left = " pgsize  " + breadcrumb(parts...)
		right = "schema: " + humanize(total)
	case viewRelations:
		var total uint64
		for _, r := range m.rels {
			total += r.SizeBytes
		}
		dbPart := fmt.Sprintf("D=%s (%s)", m.curDB, humanize(m.curDBSize()))
		schPart := fmt.Sprintf("S=%s (%s)", m.curSchema, humanize(m.curSchemaSize()))
		tblPart := fmt.Sprintf("T=%s (%s)", m.curTable, humanize(m.curTableSize()))
		parts := []string{dbPart, schPart, tblPart}
		if multi {
			parts = append([]string{m.clusterPrefix()}, parts...)
		}
		left = " pgsize  " + breadcrumb(parts...)
		right = "table: " + humanize(total)
	}
	pad := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}
	return headerStyle.Render(left) + strings.Repeat(" ", pad) + headerStyle.Render(right)
}

func (m *model) renderBody() string {
	if m.loading {
		return dimStyle.Render(" Loading...") + "\n"
	}
	switch m.view {
	case viewClusters:
		return m.renderClusters()
	case viewDatabases:
		return m.renderDatabases()
	case viewSchemas:
		return m.renderSchemas()
	case viewTables:
		return m.renderTables()
	case viewRelations:
		return m.renderRelations()
	}
	return ""
}

// pageWindow returns the [start, end) row range to display given the cursor
// position and terminal height. The column-header line and 5 chrome lines
// (app header, two separators, blank, footer) are subtracted.
func (m *model) pageWindow(n int) (start, end int) {
	maxRows := m.height - 6
	if maxRows < 1 {
		maxRows = 1
	}
	if m.cursor >= maxRows {
		start = m.cursor - maxRows + 1
	}
	end = start + maxRows
	if end > n {
		end = n
	}
	return start, end
}

func (m *model) renderClusters() string {
	n := len(m.clusters)
	start, end := m.pageWindow(n)
	var b strings.Builder
	fmtx.Fprintf(&b, "   %-s\n", "CLUSTER")
	for i := start; i < end; i++ {
		row := fmt.Sprintf(" %1s %s", cursor(i, m.cursor), m.clusters[i].Name)
		if i == m.cursor {
			row = cursorStyle.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	return b.String()
}

func (m *model) renderDatabases() string {
	var total uint64
	for _, d := range m.dbs {
		total += d.SizeBytes
	}
	sizeHdr, nameHdr := colSize, "DATABASE"
	if m.sort == sortName {
		nameHdr += " *"
	} else {
		sizeHdr += " *"
	}
	n := len(m.dbs)
	start, end := m.pageWindow(n)
	var b strings.Builder
	fmtx.Fprintf(&b, "   %-10s %5s  %-34s %s\n", sizeHdr, "%", "", nameHdr)
	for i := start; i < end; i++ {
		d := m.dbs[i]
		pct := 0.0
		if total > 0 {
			pct = float64(d.SizeBytes) / float64(total) * 100
		}
		nameW := m.width - 57
		if nameW < 1 {
			nameW = 1
		}
		row := fmt.Sprintf(" %1s %10s %5.1f  %s  %s",
			cursor(i, m.cursor), humanize(d.SizeBytes), pct, bar(pct, 32), trunc(d.Name, nameW))
		if i == m.cursor {
			row = cursorStyle.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	return b.String()
}

func (m *model) renderSchemas() string {
	var total uint64
	for _, s := range m.schs {
		total += s.SizeBytes
	}
	sizeHdr, schemaHdr := colSize, "SCHEMA"
	if m.sort == sortName {
		schemaHdr += " *"
	} else {
		sizeHdr += " *"
	}
	n := len(m.schs)
	start, end := m.pageWindow(n)
	var b strings.Builder
	fmtx.Fprintf(&b, "   %-10s %5s  %-34s %-30s %7s %5s %9s\n",
		sizeHdr, "%", "", schemaHdr, "TABLES", "IDX", "ROWS")
	for i := start; i < end; i++ {
		s := m.schs[i]
		pct := 0.0
		if total > 0 {
			pct = float64(s.SizeBytes) / float64(total) * 100
		}
		row := fmt.Sprintf(" %1s %10s %5.1f  %s  %-30s %7d %5d %9s",
			cursor(i, m.cursor), humanize(s.SizeBytes), pct, bar(pct, 32),
			trunc(s.Name, 30), s.TableCount, s.IndexCount, humanizeCount(s.RowCount))
		if i == m.cursor {
			row = cursorStyle.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	return b.String()
}

func (m *model) renderTables() string {
	var total uint64
	for _, t := range m.tbls {
		total += t.TotalBytes
	}
	sizeHdr, tableHdr := colSize, "TABLE"
	if m.sort == sortName {
		tableHdr += " *"
	} else {
		sizeHdr += " *"
	}
	nameW := min(m.width-73, 40)
	if nameW < 1 {
		nameW = 1
	}
	n := len(m.tbls)
	start, end := m.pageWindow(n)
	var b strings.Builder
	fmtx.Fprintf(
		&b,
		"   %-10s %5s  %-34s %-*s %5s %9s\n",
		sizeHdr, "%", "", nameW, tableHdr, "IDX", "ROWS",
	)
	for i := start; i < end; i++ {
		t := m.tbls[i]
		pct := 0.0
		if total > 0 {
			pct = float64(t.TotalBytes) / float64(total) * 100
		}
		row := fmt.Sprintf(
			" %1s %10s %5.1f  %s  %-*s %5d %9s",
			cursor(i, m.cursor), humanize(t.TotalBytes), pct,
			bloatBar(pct, t.BloatPct, 32), nameW, trunc(t.Name, nameW),
			len(t.Indexes), humanizeCount(t.RowCount),
		)
		if i == m.cursor {
			row = cursorStyle.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	return b.String()
}

func (m *model) renderRelations() string {
	var total uint64
	for _, r := range m.rels {
		total += r.SizeBytes
	}
	n := len(m.rels)
	start, end := m.pageWindow(n)
	var b strings.Builder
	fmtx.Fprintf(&b, "   %-10s %5s  %-34s %-8s %s\n",
		colSize, "%", "", "KIND", "NAME")

	indexHeaderShown := false
	for i := start; i < end; i++ {
		r := m.rels[i]
		isIndex := r.Kind.IsIndex()
		if isIndex && !indexHeaderShown {
			b.WriteString(dimStyle.Render(" --- indexes " + strings.Repeat("-", 60)))
			b.WriteString("\n")
			indexHeaderShown = true
		}

		pct := 0.0
		if total > 0 {
			pct = float64(r.SizeBytes) / float64(total) * 100
		}
		nameW := m.width - 67
		if nameW < 1 {
			nameW = 1
		}
		row := fmt.Sprintf(" %1s %10s %5.1f  %s  %-8s %s",
			cursor(i, m.cursor), humanize(r.SizeBytes), pct,
			bar(pct, 32), r.Kind.String(), trunc(r.Name, nameW))
		if i == m.cursor {
			row = cursorStyle.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	return b.String()
}

func (m *model) renderFooter() string {
	var hintStr string
	if m.view == viewClusters {
		hintStr = " [enter] select  [j/k] move  [q] quit"
	} else {
		sortLabel := "size"
		if m.sort == sortName {
			sortLabel = "name"
		}
		hintStr = fmt.Sprintf(
			" [enter] drill  [backspace] up  [s] sort:%s  [r] reload  [q] quit",
			sortLabel,
		)
	}
	left := dimStyle.Render(hintStr)
	if m.loading {
		left = dimStyle.Render(" loading...  [q] quit")
	}
	pos, total := m.cursorPos()
	right := dimStyle.Render(fmt.Sprintf("%d/%d ", pos, total))
	pad := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}

// helpers

func trunc(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n == 1 {
		return "~"
	}
	return string(runes[:n-1]) + "~"
}

func cursor(i, c int) string {
	if i == c {
		return ">"
	}
	return " "
}

func humanizeCount(n int64) string {
	if n <= 0 {
		return "0"
	}
	const (
		K = 1_000
		M = 1_000_000
		B = 1_000_000_000
	)
	switch {
	case n >= B:
		return fmt.Sprintf("~%.1fB", float64(n)/B)
	case n >= M:
		return fmt.Sprintf("~%.1fM", float64(n)/M)
	case n >= K:
		return fmt.Sprintf("~%.1fK", float64(n)/K)
	default:
		return fmt.Sprintf("~%d", n)
	}
}

var sizeUnits = []string{"KB", "MB", "GB", "TB", "PB"}

func humanize(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), sizeUnits[exp])
}

func bar(pct float64, width int) string {
	filled := int((pct / 100.0) * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return "[" + strings.Repeat("#", filled) + strings.Repeat(" ", width-filled) + "]"
}

func bloatBar(pct, bloatPct float64, width int) string {
	filled := int((pct / 100.0) * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	live := int(float64(filled) * (1.0 - bloatPct/100.0))
	if live < 0 {
		live = 0
	}
	if live > filled {
		live = filled
	}
	bloat := filled - live
	bloatStr := ""
	if bloat > 0 {
		bloatStr = bloatStyle.Render(strings.Repeat("!", bloat))
	}
	return "[" + strings.Repeat("#", live) + bloatStr + strings.Repeat(" ", width-filled) + "]"
}

package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/hashmap-kz/pgsize/internal/pg"
	"github.com/hashmap-kz/pgsize/internal/x/fmtx"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jackc/pgx/v5/pgxpool"
)

type viewKind int

const (
	viewDatabases viewKind = iota
	viewSchemas
	viewTables
	viewRelations
)

type sortMode int

const (
	sortSize sortMode = iota
	sortName
)

type model struct {
	pool *pgxpool.Pool // base pool, connected to the initial database
	dsn  string        // original DSN for reconnecting to a chosen database

	view   viewKind
	dbs    []pg.Database
	schs   []pg.Schema
	tbls   []pg.Table
	rels   []pg.Relation
	stack  []frame
	cursor int
	sort   sortMode

	filter      string
	filterLower string
	filterMode  bool

	curDB     string
	curSchema string
	curTable  string

	schCache map[string][]pg.Schema
	tblCache map[string][]pg.Table
	relCache map[string][]pg.Relation

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
	loadID uint64
	items  []pg.Database
	err    error
}
type loadedSchemas struct {
	loadID uint64
	db     string
	items  []pg.Schema
	err    error
}
type loadedTables struct {
	loadID uint64
	db     string
	schema string
	items  []pg.Table
	err    error
}
type loadedRelations struct {
	loadID uint64
	db     string
	schema string
	table  string
	items  []pg.Relation
	err    error
}

func InitialModel(pool *pgxpool.Pool, dbs []pg.Database, dsn string) model {
	return model{
		pool:     pool,
		dsn:      dsn,
		view:     viewDatabases,
		dbs:      dbs,
		sort:     sortSize,
		schCache: make(map[string][]pg.Schema),
		tblCache: make(map[string][]pg.Table),
		relCache: make(map[string][]pg.Relation),
	}
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.filterMode {
			return m.updateFilter(msg)
		}
		return m.updateKey(msg)

	case loadedDatabases:
		if !m.acceptLoad(msg.loadID) {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.dbs = msg.items
		m.cursor = m.firstVisibleOrZero()
		return m, nil

	case loadedSchemas:
		if !m.acceptLoad(msg.loadID) || msg.db != m.curDB {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.schs = msg.items
		m.schCache[msg.db] = msg.items
		m.view = viewSchemas
		m.cursor = m.firstVisibleOrZero()
		return m, nil

	case loadedTables:
		if !m.acceptLoad(msg.loadID) || msg.db != m.curDB || msg.schema != m.curSchema {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.tbls = msg.items
		m.tblCache[tableCacheKey(msg.db, msg.schema)] = msg.items
		m.view = viewTables
		m.cursor = m.firstVisibleOrZero()
		return m, nil

	case loadedRelations:
		if !m.acceptLoad(msg.loadID) || msg.db != m.curDB || msg.schema != m.curSchema || msg.table != m.curTable {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.rels = msg.items
		m.relCache[relationCacheKey(msg.db, msg.schema, msg.table)] = msg.items
		m.view = viewRelations
		m.cursor = m.firstVisibleOrZero()
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
		case "backspace", "h", "left":
			m.err = nil
		}
		return m, nil
	}
	switch msg.String() {
	case "j", "down":
		m.moveVisible(1)
	case "k", "up":
		m.moveVisible(-1)
	case "g", "home":
		m.cursor = m.firstVisibleOrZero()
	case "G", "end":
		m.cursor = m.lastVisibleOrZero()
	case "enter", "l", "right":
		return m, m.drillIn()
	case "backspace", "h", "left":
		m.drillOut()
	case "s":
		if m.sort == sortSize {
			m.sort = sortName
		} else {
			m.sort = sortSize
		}
		m.applySort()
		m.cursor = m.firstVisibleOrZero()
	case "r":
		return m, m.reload()
	case "/":
		m.filterMode = true
		m.filter = ""
		m.filterLower = ""
		m.cursor = m.firstVisibleOrZero()
	}
	return m, nil
}

func (m *model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.loading {
		return m, nil
	}
	switch msg.String() {
	case "esc", "enter":
		m.filterMode = false
	case "backspace":
		if m.filter != "" {
			_, size := utf8.DecodeLastRuneInString(m.filter)
			m.filter = m.filter[:len(m.filter)-size]
			m.filterLower = strings.ToLower(m.filter)
			m.cursor = m.firstVisibleOrZero()
		}
	default:
		if s := msg.String(); utf8.RuneCountInString(s) == 1 {
			m.filter += s
			m.filterLower = strings.ToLower(m.filter)
			m.cursor = m.firstVisibleOrZero()
		}
	}
	return m, nil
}

func (m *model) rowCount() int {
	switch m.view {
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

func (m *model) matchAt(i int) bool {
	switch m.view {
	case viewDatabases:
		if i >= 0 && i < len(m.dbs) {
			return match(m.dbs[i].Name, m.filterLower)
		}
	case viewSchemas:
		if i >= 0 && i < len(m.schs) {
			return match(m.schs[i].Name, m.filterLower)
		}
	case viewTables:
		if i >= 0 && i < len(m.tbls) {
			return match(m.tbls[i].Name, m.filterLower)
		}
	case viewRelations:
		if i >= 0 && i < len(m.rels) {
			return match(m.rels[i].Name, m.filterLower)
		}
	}
	return false
}

func (m *model) visibleIndexes() []int {
	n := m.rowCount()
	out := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if m.matchAt(i) {
			out = append(out, i)
		}
	}
	return out
}

func (m *model) firstVisibleOrZero() int {
	visible := m.visibleIndexes()
	if len(visible) == 0 {
		return 0
	}
	return visible[0]
}

func (m *model) lastVisibleOrZero() int {
	visible := m.visibleIndexes()
	if len(visible) == 0 {
		return 0
	}
	return visible[len(visible)-1]
}

func (m *model) visibleCursorOrFirst() int {
	if m.matchAt(m.cursor) {
		return m.cursor
	}
	return m.firstVisibleOrZero()
}

func (m *model) moveVisible(delta int) {
	visible := m.visibleIndexes()
	if len(visible) == 0 {
		m.cursor = 0
		return
	}
	pos := -1
	for i, idx := range visible {
		if idx == m.cursor {
			pos = i
			break
		}
	}
	if pos == -1 {
		m.cursor = visible[0]
		return
	}
	pos += delta
	if pos < 0 {
		pos = 0
	}
	if pos >= len(visible) {
		pos = len(visible) - 1
	}
	m.cursor = visible[pos]
}

func (m *model) filteredPos() (pos, total int) {
	visible := m.visibleIndexes()
	total = len(visible)
	for i, idx := range visible {
		if idx == m.cursor {
			return i + 1, total
		}
	}
	return 0, total
}

func (m *model) drillIn() tea.Cmd {
	if m.rowCount() == 0 || !m.matchAt(m.cursor) {
		return nil
	}
	f := frame{
		view: m.view, cursor: m.cursor,
		curDB: m.curDB, curSch: m.curSchema, curTbl: m.curTable,
	}
	m.filter = ""
	m.filterLower = ""
	m.filterMode = false
	switch m.view {
	case viewDatabases:
		dbName := m.dbs[m.cursor].Name
		m.stack = append(m.stack, f)
		m.curDB = dbName
		m.curSchema = ""
		m.curTable = ""
		if schs, ok := m.schCache[dbName]; ok {
			m.schs = schs
			m.view = viewSchemas
			m.cursor = m.firstVisibleOrZero()
			return nil
		}
		m.loading = true
		loadID := m.nextLoadID()
		dsn := m.dsn
		return func() tea.Msg {
			items, err := withDatabasePool(context.Background(), dsn, dbName, pg.ListSchemas)
			return loadedSchemas{loadID: loadID, db: dbName, items: items, err: err}
		}
	case viewSchemas:
		schemaName := m.schs[m.cursor].Name
		m.stack = append(m.stack, f)
		m.curSchema = schemaName
		m.curTable = ""
		cacheKey := tableCacheKey(m.curDB, schemaName)
		if tbls, ok := m.tblCache[cacheKey]; ok {
			m.tbls = tbls
			m.view = viewTables
			m.cursor = m.firstVisibleOrZero()
			return nil
		}
		m.loading = true
		loadID := m.nextLoadID()
		dsn, dbName := m.dsn, m.curDB
		return func() tea.Msg {
			items, err := withDatabasePool(context.Background(), dsn, dbName, func(ctx context.Context, pool *pgxpool.Pool) ([]pg.Table, error) {
				return pg.ListTables(ctx, pool, schemaName)
			})
			return loadedTables{loadID: loadID, db: dbName, schema: schemaName, items: items, err: err}
		}
	case viewTables:
		tableName := m.tbls[m.cursor].Name
		m.stack = append(m.stack, f)
		m.curTable = tableName
		cacheKey := relationCacheKey(m.curDB, m.curSchema, tableName)
		if rels, ok := m.relCache[cacheKey]; ok {
			m.rels = rels
			m.view = viewRelations
			m.cursor = m.firstVisibleOrZero()
			return nil
		}
		m.loading = true
		loadID := m.nextLoadID()
		dsn, dbName, schemaName := m.dsn, m.curDB, m.curSchema
		return func() tea.Msg {
			items, err := withDatabasePool(context.Background(), dsn, dbName, func(ctx context.Context, pool *pgxpool.Pool) ([]pg.Relation, error) {
				return pg.ListRelations(ctx, pool, schemaName, tableName)
			})
			return loadedRelations{loadID: loadID, db: dbName, schema: schemaName, table: tableName, items: items, err: err}
		}
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
	m.filter = ""
	m.filterLower = ""
	m.filterMode = false
	m.cursor = m.visibleCursorOrFirst()
}

func (m *model) reload() tea.Cmd {
	if m.loading {
		return nil
	}
	m.loading = true
	loadID := m.nextLoadID()
	switch m.view {
	case viewDatabases:
		pool := m.pool
		return func() tea.Msg {
			items, err := pg.ListDatabases(context.Background(), pool)
			return loadedDatabases{loadID: loadID, items: items, err: err}
		}
	case viewSchemas:
		dbName := m.curDB
		delete(m.schCache, dbName)
		dsn := m.dsn
		return func() tea.Msg {
			items, err := withDatabasePool(context.Background(), dsn, dbName, pg.ListSchemas)
			return loadedSchemas{loadID: loadID, db: dbName, items: items, err: err}
		}
	case viewTables:
		dbName, schemaName := m.curDB, m.curSchema
		delete(m.tblCache, tableCacheKey(dbName, schemaName))
		dsn := m.dsn
		return func() tea.Msg {
			items, err := withDatabasePool(context.Background(), dsn, dbName, func(ctx context.Context, pool *pgxpool.Pool) ([]pg.Table, error) {
				return pg.ListTables(ctx, pool, schemaName)
			})
			return loadedTables{loadID: loadID, db: dbName, schema: schemaName, items: items, err: err}
		}
	case viewRelations:
		dbName, schemaName, tableName := m.curDB, m.curSchema, m.curTable
		delete(m.relCache, relationCacheKey(dbName, schemaName, tableName))
		dsn := m.dsn
		return func() tea.Msg {
			items, err := withDatabasePool(context.Background(), dsn, dbName, func(ctx context.Context, pool *pgxpool.Pool) ([]pg.Relation, error) {
				return pg.ListRelations(ctx, pool, schemaName, tableName)
			})
			return loadedRelations{loadID: loadID, db: dbName, schema: schemaName, table: tableName, items: items, err: err}
		}
	}
	return nil
}

func withDatabasePool[T any](ctx context.Context, dsn, dbName string, fn func(context.Context, *pgxpool.Pool) ([]T, error)) ([]T, error) {
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
	return db + "\x00" + schema
}

func relationCacheKey(db, schema, table string) string {
	return db + "\x00" + schema + "\x00" + table
}

func (m *model) applySort() {
	switch m.view {
	case viewDatabases:
		if m.sort == sortSize {
			sort.Slice(m.dbs, func(i, j int) bool { return m.dbs[i].SizeBytes > m.dbs[j].SizeBytes })
		} else {
			sort.Slice(m.dbs, func(i, j int) bool { return m.dbs[i].Name < m.dbs[j].Name })
		}
	case viewSchemas:
		if m.sort == sortSize {
			sort.Slice(m.schs, func(i, j int) bool { return m.schs[i].SizeBytes > m.schs[j].SizeBytes })
		} else {
			sort.Slice(m.schs, func(i, j int) bool { return m.schs[i].Name < m.schs[j].Name })
		}
	case viewTables:
		if m.sort == sortSize {
			sort.Slice(m.tbls, func(i, j int) bool { return m.tbls[i].TotalBytes > m.tbls[j].TotalBytes })
		} else {
			sort.Slice(m.tbls, func(i, j int) bool { return m.tbls[i].Name < m.tbls[j].Name })
		}
	}
}

// rendering

var (
	headerStyle = lipgloss.NewStyle().Bold(true)
	cursorStyle = lipgloss.NewStyle().Bold(true)
	dimStyle    = lipgloss.NewStyle().Faint(true)
)

func (m *model) View() string {
	if m.err != nil {
		return fmt.Sprintf("error: %v\n\n [backspace] dismiss  [q] quit", m.err)
	}

	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", m.width))
	b.WriteString("\n")
	b.WriteString(m.renderBody())
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", m.width))
	b.WriteString("\n")
	b.WriteString(m.renderFooter())
	return b.String()
}

func (m *model) renderHeader() string {
	var left, right string
	switch m.view {
	case viewDatabases:
		var total uint64
		for _, d := range m.dbs {
			total += d.SizeBytes
		}
		left = " pgsize"
		right = "total: " + humanize(total)
	case viewSchemas:
		var total uint64
		for _, s := range m.schs {
			total += s.SizeBytes
		}
		left = " pgsize  " + m.curDB
		right = "db: " + humanize(total)
	case viewTables:
		var total uint64
		for _, t := range m.tbls {
			total += t.TotalBytes
		}
		left = fmt.Sprintf(" pgsize  %s  %s", m.curDB, m.curSchema)
		right = "schema: " + humanize(total)
	case viewRelations:
		var total uint64
		for _, r := range m.rels {
			total += r.SizeBytes
		}
		left = fmt.Sprintf(" pgsize  %s  %s  %s", m.curDB, m.curSchema, m.curTable)
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

// pageWindow returns the slice [start, end) of visible items to show given the
// cursor position and terminal height. The column-header line and 5 chrome
// lines (app header, two separators, blank, footer) are subtracted.
func (m *model) pageWindow(visible []int) (start, end int) {
	maxRows := m.height - 6
	if maxRows < 1 {
		maxRows = 1
	}
	cursorPos := 0
	for i, idx := range visible {
		if idx == m.cursor {
			cursorPos = i
			break
		}
	}
	if cursorPos >= maxRows {
		start = cursorPos - maxRows + 1
	}
	end = start + maxRows
	if end > len(visible) {
		end = len(visible)
	}
	return start, end
}

func (m *model) renderDatabases() string {
	var total uint64
	for _, d := range m.dbs {
		total += d.SizeBytes
	}
	sizeHdr, nameHdr := "SIZE", "NAME"
	if m.sort == sortName {
		nameHdr += " *"
	} else {
		sizeHdr += " *"
	}
	visible := m.visibleIndexes()
	start, end := m.pageWindow(visible)
	var b strings.Builder
	fmtx.Fprintf(&b, "   %-10s %5s  %-34s %s\n", sizeHdr, "%", "", nameHdr)
	for _, i := range visible[start:end] {
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
	if len(visible) == 0 && m.filterLower != "" {
		b.WriteString(dimStyle.Render("  no matches") + "\n")
	}
	return b.String()
}

func (m *model) renderSchemas() string {
	var total uint64
	for _, s := range m.schs {
		total += s.SizeBytes
	}
	sizeHdr, schemaHdr := "SIZE", "SCHEMA"
	if m.sort == sortName {
		schemaHdr += " *"
	} else {
		sizeHdr += " *"
	}
	visible := m.visibleIndexes()
	start, end := m.pageWindow(visible)
	var b strings.Builder
	fmtx.Fprintf(&b, "   %-10s %5s  %-34s %-30s %7s %5s %9s\n",
		sizeHdr, "%", "", schemaHdr, "TABLES", "IDX", "ROWS")
	for _, i := range visible[start:end] {
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
	if len(visible) == 0 && m.filterLower != "" {
		b.WriteString(dimStyle.Render("  no matches") + "\n")
	}
	return b.String()
}

func (m *model) renderTables() string {
	var total uint64
	for _, t := range m.tbls {
		total += t.TotalBytes
	}
	sizeHdr, tableHdr := "SIZE", "TABLE"
	if m.sort == sortName {
		tableHdr += " *"
	} else {
		sizeHdr += " *"
	}
	nameW := m.width - 73
	if nameW < 1 {
		nameW = 1
	}
	visible := m.visibleIndexes()
	start, end := m.pageWindow(visible)
	var b strings.Builder
	fmtx.Fprintf(&b, "   %-10s %5s  %-34s %-*s %5s %9s\n", sizeHdr, "%", "", nameW, tableHdr, "IDX", "ROWS")
	for _, i := range visible[start:end] {
		t := m.tbls[i]
		pct := 0.0
		if total > 0 {
			pct = float64(t.TotalBytes) / float64(total) * 100
		}
		row := fmt.Sprintf(" %1s %10s %5.1f  %s  %-*s %5d %9s",
			cursor(i, m.cursor), humanize(t.TotalBytes), pct, bar(pct, 32), nameW, trunc(t.Name, nameW), len(t.Indexes), humanizeCount(t.RowCount))
		if i == m.cursor {
			row = cursorStyle.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	if len(visible) == 0 && m.filterLower != "" {
		b.WriteString(dimStyle.Render("  no matches") + "\n")
	}
	return b.String()
}

func (m *model) renderRelations() string {
	var total uint64
	for _, r := range m.rels {
		total += r.SizeBytes
	}
	visible := m.visibleIndexes()
	start, end := m.pageWindow(visible)
	var b strings.Builder
	fmtx.Fprintf(&b, "   %-10s %5s  %-34s %-8s %s\n",
		"SIZE", "%", "", "KIND", "NAME")

	indexHeaderShown := false
	for _, i := range visible[start:end] {
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
	if len(visible) == 0 && m.filterLower != "" {
		b.WriteString(dimStyle.Render("  no matches") + "\n")
	}
	return b.String()
}

func (m *model) renderFooter() string {
	if m.filterMode {
		return "/" + m.filter
	}
	sortLabel := "size"
	if m.sort == sortName {
		sortLabel = "name"
	}
	left := dimStyle.Render(fmt.Sprintf(" [enter] drill  [backspace] up  [s] sort:%s  [/] filter  [r] reload  [q] quit", sortLabel))
	if m.loading {
		left = dimStyle.Render(" loading...  [q] quit")
	}
	pos, total := m.filteredPos()
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

func match(name, filterLower string) bool {
	if filterLower == "" {
		return true
	}
	return strings.Contains(strings.ToLower(name), filterLower)
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

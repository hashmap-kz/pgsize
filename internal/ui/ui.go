package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hashmap-kz/pgsize/internal/pg"

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
	pool   *pgxpool.Pool // base pool (always connected to initial DB)
	dbPool *pgxpool.Pool // pool for the currently-selected database; nil at root
	dsn    string        // original DSN for reconnecting to a chosen database

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

	schCache  map[string][]pg.Schema
	tblCache  map[string][]pg.Table
	relCache  map[string][]pg.Relation
	poolCache map[string]*pgxpool.Pool

	loading bool

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
	items []pg.Database
	err   error
}
type loadedSchemas struct {
	items []pg.Schema
	pool  *pgxpool.Pool // non-nil when a new per-database pool was created
	err   error
}
type loadedTables struct {
	items []pg.Table
	err   error
}
type loadedRelations struct {
	items []pg.Relation
	err   error
}

func InitialModel(pool *pgxpool.Pool, dbs []pg.Database, dsn string) model {
	return model{
		pool:      pool,
		dsn:       dsn,
		view:      viewDatabases,
		dbs:       dbs,
		sort:      sortSize,
		schCache:  make(map[string][]pg.Schema),
		tblCache:  make(map[string][]pg.Table),
		relCache:  make(map[string][]pg.Relation),
		poolCache: make(map[string]*pgxpool.Pool),
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.dbs = msg.items
		return m, nil

	case loadedSchemas:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if msg.pool != nil {
			m.dbPool = msg.pool
			m.poolCache[m.curDB] = msg.pool
		}
		m.schs = msg.items
		m.schCache[m.curDB] = msg.items
		m.view = viewSchemas
		m.cursor = 0
		return m, nil

	case loadedTables:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.tbls = msg.items
		m.tblCache[m.curDB+"\x00"+m.curSchema] = msg.items
		m.view = viewTables
		m.cursor = 0
		return m, nil

	case loadedRelations:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.rels = msg.items
		m.relCache[m.curDB+"\x00"+m.curSchema+"\x00"+m.curTable] = msg.items
		m.view = viewRelations
		m.cursor = 0
		return m, nil
	}
	return m, nil
}

func (m model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "q" || msg.String() == "ctrl+c" {
		return m, tea.Quit
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
		if next := m.nextMatch(m.cursor+1, 1); next >= 0 {
			m.cursor = next
		}
	case "k", "up":
		if prev := m.nextMatch(m.cursor-1, -1); prev >= 0 {
			m.cursor = prev
		}
	case "g", "home":
		if first := m.nextMatch(0, 1); first >= 0 {
			m.cursor = first
		}
	case "G", "end":
		if last := m.nextMatch(m.rowCount()-1, -1); last >= 0 {
			m.cursor = last
		}
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
	case "r":
		return m, m.reload()
	case "/":
		m.filterMode = true
		m.filter = ""
		m.filterLower = ""
	}
	return m, nil
}

func (m model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter":
		m.filterMode = false
	case "backspace":
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.filterLower = strings.ToLower(m.filter)
			if !m.matchAt(m.cursor) {
				if first := m.nextMatch(0, 1); first >= 0 {
					m.cursor = first
				}
			}
		}
	default:
		if len(msg.String()) == 1 {
			m.filter += msg.String()
			m.filterLower = strings.ToLower(m.filter)
			if !m.matchAt(m.cursor) {
				if first := m.nextMatch(0, 1); first >= 0 {
					m.cursor = first
				}
			}
		}
	}
	return m, nil
}

func (m model) rowCount() int {
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

func (m model) matchAt(i int) bool {
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

func (m model) nextMatch(start, dir int) int {
	n := m.rowCount()
	for i := start; i >= 0 && i < n; i += dir {
		if m.matchAt(i) {
			return i
		}
	}
	return -1
}

func (m model) filteredPos() (pos, total int) {
	n := m.rowCount()
	for i := 0; i < n; i++ {
		if m.matchAt(i) {
			total++
			if i == m.cursor {
				pos = total
			}
		}
	}
	return pos, total
}

// queryPool returns the pool to use for schema/table/relation queries.
func (m model) queryPool() *pgxpool.Pool {
	if m.dbPool != nil {
		return m.dbPool
	}
	return m.pool
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
	case viewDatabases:
		m.stack = append(m.stack, f)
		dbName := m.dbs[m.cursor].Name
		m.curDB = dbName
		if schs, ok := m.schCache[dbName]; ok {
			m.dbPool = m.poolCache[dbName]
			m.schs = schs
			m.view = viewSchemas
			m.cursor = 0
			return nil
		}
		m.loading = true
		dsn := m.dsn
		return func() tea.Msg {
			cfg, err := pgxpool.ParseConfig(dsn)
			if err != nil {
				return loadedSchemas{err: err}
			}
			cfg.ConnConfig.Database = dbName
			pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
			if err != nil {
				return loadedSchemas{err: err}
			}
			items, err := pg.ListSchemas(context.Background(), pool)
			if err != nil {
				pool.Close()
				return loadedSchemas{err: err}
			}
			return loadedSchemas{items: items, pool: pool}
		}
	case viewSchemas:
		m.stack = append(m.stack, f)
		m.curSchema = m.schs[m.cursor].Name
		cacheKey := m.curDB + "\x00" + m.curSchema
		if tbls, ok := m.tblCache[cacheKey]; ok {
			m.tbls = tbls
			m.view = viewTables
			m.cursor = 0
			return nil
		}
		m.loading = true
		pool, sch := m.queryPool(), m.curSchema
		return func() tea.Msg {
			items, err := pg.ListTables(context.Background(), pool, sch)
			return loadedTables{items: items, err: err}
		}
	case viewTables:
		m.stack = append(m.stack, f)
		m.curTable = m.tbls[m.cursor].Name
		cacheKey := m.curDB + "\x00" + m.curSchema + "\x00" + m.curTable
		if rels, ok := m.relCache[cacheKey]; ok {
			m.rels = rels
			m.view = viewRelations
			m.cursor = 0
			return nil
		}
		m.loading = true
		pool, sch, tbl := m.queryPool(), m.curSchema, m.curTable
		return func() tea.Msg {
			items, err := pg.ListRelations(context.Background(), pool, sch, tbl)
			return loadedRelations{items: items, err: err}
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
	if f.view == viewDatabases {
		m.dbPool = nil
	}
	m.view = f.view
	m.cursor = f.cursor
	m.curDB = f.curDB
	m.curSchema = f.curSch
	m.curTable = f.curTbl
}

func (m *model) reload() tea.Cmd {
	m.loading = true
	switch m.view {
	case viewDatabases:
		pool := m.pool
		return func() tea.Msg {
			items, err := pg.ListDatabases(context.Background(), pool)
			return loadedDatabases{items: items, err: err}
		}
	case viewSchemas:
		delete(m.schCache, m.curDB)
		pool := m.queryPool()
		return func() tea.Msg {
			items, err := pg.ListSchemas(context.Background(), pool)
			return loadedSchemas{items: items, err: err}
		}
	case viewTables:
		delete(m.tblCache, m.curDB+"\x00"+m.curSchema)
		pool, sch := m.queryPool(), m.curSchema
		return func() tea.Msg {
			items, err := pg.ListTables(context.Background(), pool, sch)
			return loadedTables{items: items, err: err}
		}
	case viewRelations:
		delete(m.relCache, m.curDB+"\x00"+m.curSchema+"\x00"+m.curTable)
		pool, sch, tbl := m.queryPool(), m.curSchema, m.curTable
		return func() tea.Msg {
			items, err := pg.ListRelations(context.Background(), pool, sch, tbl)
			return loadedRelations{items: items, err: err}
		}
	}
	return nil
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

func (m model) View() string {
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

func (m model) renderHeader() string {
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

func (m model) renderBody() string {
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

// pageWindow returns the slice [start, end) of n items to show given the
// cursor position and terminal height. The column-header line and 5 chrome
// lines (app header, two separators, blank, footer) are subtracted.
func (m model) pageWindow(n int) (start, end int) {
	maxRows := m.height - 6
	if maxRows < 1 {
		maxRows = 1
	}
	start = 0
	if m.cursor >= maxRows {
		start = m.cursor - maxRows + 1
	}
	end = start + maxRows
	if end > n {
		end = n
	}
	return start, end
}

func (m model) renderDatabases() string {
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
	start, end := m.pageWindow(len(m.dbs))
	var b strings.Builder
	fmt.Fprintf(&b, "   %-10s %5s  %-34s %s\n", sizeHdr, "%", "", nameHdr)
	shown := 0
	for i := start; i < end; i++ {
		d := m.dbs[i]
		if !match(d.Name, m.filterLower) {
			continue
		}
		shown++
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
	if shown == 0 && m.filterLower != "" {
		b.WriteString(dimStyle.Render("  no matches") + "\n")
	}
	return b.String()
}

func (m model) renderSchemas() string {
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
	start, end := m.pageWindow(len(m.schs))
	var b strings.Builder
	fmt.Fprintf(&b, "   %-10s %5s  %-34s %-30s %7s %5s\n",
		sizeHdr, "%", "", schemaHdr, "TABLES", "IDX")
	shown := 0
	for i := start; i < end; i++ {
		s := m.schs[i]
		if !match(s.Name, m.filterLower) {
			continue
		}
		shown++
		pct := 0.0
		if total > 0 {
			pct = float64(s.SizeBytes) / float64(total) * 100
		}
		row := fmt.Sprintf(" %1s %10s %5.1f  %s  %-30s %7d %5d",
			cursor(i, m.cursor), humanize(s.SizeBytes), pct, bar(pct, 32),
			trunc(s.Name, 30), s.TableCount, s.IndexCount)
		if i == m.cursor {
			row = cursorStyle.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	if shown == 0 && m.filterLower != "" {
		b.WriteString(dimStyle.Render("  no matches") + "\n")
	}
	return b.String()
}

func (m model) renderTables() string {
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
	nameW := m.width - 63
	if nameW < 1 {
		nameW = 1
	}
	start, end := m.pageWindow(len(m.tbls))
	var b strings.Builder
	fmt.Fprintf(&b, "   %-10s %5s  %-34s %-*s %5s\n", sizeHdr, "%", "", nameW, tableHdr, "IDX")
	shown := 0
	for i := start; i < end; i++ {
		t := m.tbls[i]
		if !match(t.Name, m.filterLower) {
			continue
		}
		shown++
		pct := 0.0
		if total > 0 {
			pct = float64(t.TotalBytes) / float64(total) * 100
		}
		row := fmt.Sprintf(" %1s %10s %5.1f  %s  %-*s %5d",
			cursor(i, m.cursor), humanize(t.TotalBytes), pct, bar(pct, 32), nameW, trunc(t.Name, nameW), len(t.Indexes))
		if i == m.cursor {
			row = cursorStyle.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	if shown == 0 && m.filterLower != "" {
		b.WriteString(dimStyle.Render("  no matches") + "\n")
	}
	return b.String()
}

func (m model) renderRelations() string {
	var total uint64
	for _, r := range m.rels {
		total += r.SizeBytes
	}
	start, end := m.pageWindow(len(m.rels))
	var b strings.Builder
	fmt.Fprintf(&b, "   %-10s %5s  %-34s %-8s %s\n",
		"SIZE", "%", "", "KIND", "NAME")

	shown := 0
	indexHeaderShown := false
	for i := start; i < end; i++ {
		r := m.rels[i]
		if !match(r.Name, m.filterLower) {
			continue
		}
		shown++

		isIndex := r.Kind != pg.RelHeap && r.Kind != pg.RelToast && r.Kind != pg.RelFsmVm
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
	if shown == 0 && m.filterLower != "" {
		b.WriteString(dimStyle.Render("  no matches") + "\n")
	}
	return b.String()
}

func (m model) renderFooter() string {
	if m.filterMode {
		return "/" + m.filter
	}
	sortLabel := "size"
	if m.sort == sortName {
		sortLabel = "name"
	}
	left := dimStyle.Render(fmt.Sprintf(" [enter] drill  [backspace] up  [s] sort:%s  [/] filter  [r] reload  [q] quit", sortLabel))
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
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "~"
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

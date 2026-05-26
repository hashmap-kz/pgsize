package ui

import (
	"context"
	"fmt"
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

	filter     string
	filterMode bool

	curDB     string
	curSchema string
	curTable  string

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
		pool: pool,
		dsn:  dsn,
		view: viewDatabases,
		dbs:  dbs,
		sort: sortSize,
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

	case loadedSchemas:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if m.dbPool != nil {
			m.dbPool.Close()
		}
		m.dbPool = msg.pool
		m.schs = msg.items
		m.view = viewSchemas
		m.cursor = 0
		return m, nil

	case loadedTables:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.tbls = msg.items
		m.view = viewTables
		m.cursor = 0
		return m, nil

	case loadedRelations:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.rels = msg.items
		m.view = viewRelations
		m.cursor = 0
		return m, nil
	}
	return m, nil
}

func (m model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		if m.cursor < m.rowCount()-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = m.rowCount() - 1
		if m.cursor < 0 {
			m.cursor = 0
		}
	case "enter", "l", "right":
		cmd := m.drillIn()
		return m, cmd
	case "backspace", "h", "left":
		m.drillOut()
	case "s":
		if m.sort == sortSize {
			m.sort = sortName
		} else {
			m.sort = sortSize
		}
		m.applySort()
	case "/":
		m.filterMode = true
		m.filter = ""
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
		}
	default:
		if len(msg.String()) == 1 {
			m.filter += msg.String()
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
		pool, sch := m.queryPool(), m.curSchema
		return func() tea.Msg {
			items, err := pg.ListTables(context.Background(), pool, sch)
			return loadedTables{items: items, err: err}
		}
	case viewTables:
		m.stack = append(m.stack, f)
		m.curTable = m.tbls[m.cursor].Name
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
	// Going back to databases: release the per-database pool.
	if f.view == viewDatabases {
		if m.dbPool != nil {
			m.dbPool.Close()
			m.dbPool = nil
		}
	}
	m.view = f.view
	m.cursor = f.cursor
	m.curDB = f.curDB
	m.curSchema = f.curSch
	m.curTable = f.curTbl
}

func (m *model) applySort() {
	// left as a sketch: sort the active slice by size or name
}

// rendering

var (
	headerStyle = lipgloss.NewStyle().Bold(true)
	cursorStyle = lipgloss.NewStyle().Bold(true)
	dimStyle    = lipgloss.NewStyle().Faint(true)
)

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("error: %v\n[q] quit", m.err)
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
	start, end := m.pageWindow(len(m.dbs))
	var b strings.Builder
	fmt.Fprintf(&b, "   %-10s %5s  %-34s %s\n", "SIZE", "%", "", "NAME")
	for i := start; i < end; i++ {
		d := m.dbs[i]
		if !match(d.Name, m.filter) {
			continue
		}
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

func (m model) renderSchemas() string {
	var total uint64
	for _, s := range m.schs {
		total += s.SizeBytes
	}
	start, end := m.pageWindow(len(m.schs))
	var b strings.Builder
	fmt.Fprintf(&b, "   %-10s %5s  %-34s %-30s %7s %5s\n",
		"SIZE", "%", "", "SCHEMA", "TABLES", "IDX")
	for i := start; i < end; i++ {
		s := m.schs[i]
		if !match(s.Name, m.filter) {
			continue
		}
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
	return b.String()
}

func (m model) renderTables() string {
	var total uint64
	for _, t := range m.tbls {
		total += t.TotalBytes
	}
	start, end := m.pageWindow(len(m.tbls))
	var b strings.Builder
	fmt.Fprintf(&b, "   %-10s %5s  %-34s %s\n", "SIZE", "%", "", "TABLE")
	for i := start; i < end; i++ {
		t := m.tbls[i]
		if !match(t.Name, m.filter) {
			continue
		}
		pct := 0.0
		if total > 0 {
			pct = float64(t.TotalBytes) / float64(total) * 100
		}
		nameW := m.width - 57
		if nameW < 1 {
			nameW = 1
		}
		row := fmt.Sprintf(" %1s %10s %5.1f  %s  %s",
			cursor(i, m.cursor), humanize(t.TotalBytes), pct, bar(pct, 32), trunc(t.Name, nameW))
		if i == m.cursor {
			row = cursorStyle.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
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

	indexHeaderShown := false
	for i := start; i < end; i++ {
		r := m.rels[i]
		if !match(r.Name, m.filter) {
			continue
		}

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
	return b.String()
}

func (m model) renderFooter() string {
	if m.filterMode {
		return "/" + m.filter
	}
	return dimStyle.Render(" [enter] drill  [backspace] up  [s] sort  [/] filter  [q] quit")
}

// helpers

// trunc clips s to at most n display columns, appending "…" when cut.
func trunc(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

func cursor(i, c int) string {
	if i == c {
		return ">"
	}
	return " "
}

func match(name, filter string) bool {
	if filter == "" {
		return true
	}
	return strings.Contains(strings.ToLower(name), strings.ToLower(filter))
}

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
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), units[exp])
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

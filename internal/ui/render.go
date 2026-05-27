package ui

import (
	"fmt"
	"strings"

	"github.com/hashmap-kz/pgsize/internal/x/fmtx"

	"github.com/charmbracelet/lipgloss"
)

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

const (
	// barWidth is the inner character width of the ASCII progress bar (excluding brackets).
	barWidth = 32

	// colsPrefix is the total fixed-column character width preceding the name column
	// in all data rows. 25 accounts for all non-bar fixed chars; bar occupies barWidth+2
	// (inner width plus brackets).
	colsPrefix = barWidth + 25

	// colsTableSuffix is the fixed trailing column width shared by the tables/schemas views
	// (idx and rows columns with their separating spaces).
	colsTableSuffix = 16

	// colKindWidth is the character width of the relation-kind column.
	colKindWidth = 8

	// fixedColsDB is the total fixed-column width for the databases view.
	// The name column is last, so only colsPrefix is fixed.
	fixedColsDB = colsPrefix

	// fixedColsTables is the total fixed-column width for the tables view.
	fixedColsTables = colsPrefix + colsTableSuffix

	// fixedColsRelations is the total fixed-column width for the relations view.
	fixedColsRelations = colsPrefix + colKindWidth + 2

	// nameWidthSchemas is the fixed name-column character width in the schemas view.
	nameWidthSchemas = 30

	// nameWidthTablesMax caps the name-column character width in the tables view.
	nameWidthTablesMax = 40

	// chromeLinesView is the number of fixed UI chrome rows subtracted when padding
	// the body to terminal height in renderView.
	chromeLinesView = 5

	// chromeLinesPage is the number of fixed UI chrome rows subtracted when computing
	// the scrollable page window in pageWindow.
	chromeLinesPage = 6

	// indexSepWidth is the dash count in the index-group separator within the relations view.
	indexSepWidth = 60
)

// headerFmt holds the column header labels that mirror the rowFmt layout.
// The common prefix occupies the same character positions as rowFmt's fixed
// columns, so headers and data rows align without manual spacing.
type headerFmt struct {
	size   string // size column label (may carry " *" sort indicator)
	name   string // name column label (may carry " *" sort indicator)
	nameW  int    // name-column width; must match the data rows' nameW
	extras string // pre-formatted trailing column labels (optional)
}

// String renders the header row.
func (h headerFmt) String() string {
	base := fmt.Sprintf("   %-10s %5s  %-34s %-*s",
		h.size, "%", "", h.nameW, h.name)
	if h.extras != "" {
		return base + h.extras
	}
	return base
}

// rowFmt holds the display values for every column in a data row.
// Using a struct instead of a long positional fmt.Sprintf arg list makes each
// column mapping explicit and prevents off-by-one mistakes when reading or
// modifying the layout.
type rowFmt struct {
	cur    string  // cursor symbol, from cursor()
	size   string  // humanized byte count
	pct    float64 // percentage of the view total
	bar    string  // rendered progress bar, from bar() or bloatBar()
	name   string  // item name, already truncated to nameW
	nameW  int     // name-column width for %-* left-alignment
	extras string  // pre-formatted view-specific trailing columns (optional)
}

// String renders all columns: the six common ones followed by extras if present.
func (r rowFmt) String() string {
	base := fmt.Sprintf(" %1s %10s %5.1f  %s  %-*s",
		r.cur, r.size, r.pct, r.bar, r.nameW, r.name)
	if r.extras != "" {
		return base + r.extras
	}
	return base
}

func (m *model) renderView() string {
	if m.err != nil {
		return fmt.Sprintf("error: %v\n\n [backspace] dismiss  [q] quit", m.err)
	}

	body := m.renderBody()
	if m.height > chromeLinesView {
		if need := m.height - chromeLinesView - strings.Count(body, "\n"); need > 0 {
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
	pad := max(m.width-lipgloss.Width(left)-lipgloss.Width(right), 1)
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
// position and terminal height. The column-header line and chromeLinesPage chrome
// lines (app header, two separators, blank, footer) are subtracted.
func (m *model) pageWindow(n int) (start, end int) {
	maxRows := max(m.height-chromeLinesPage, 1)
	if m.cursor >= maxRows {
		start = m.cursor - maxRows + 1
	}
	end = min(start+maxRows, n)
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
	nameW := max(m.width-fixedColsDB, 1)
	n := len(m.dbs)
	start, end := m.pageWindow(n)
	var b strings.Builder
	fmtx.Fprintln(&b, headerFmt{size: sizeHdr, name: nameHdr, nameW: nameW}.String())
	for i := start; i < end; i++ {
		d := m.dbs[i]
		pct := 0.0
		if total > 0 {
			pct = float64(d.SizeBytes) / float64(total) * 100
		}
		row := rowFmt{
			cur:   cursor(i, m.cursor),
			size:  humanize(d.SizeBytes),
			pct:   pct,
			bar:   bar(pct, barWidth),
			name:  trunc(d.Name, nameW),
			nameW: nameW,
		}.String()
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
	fmtx.Fprintln(&b, headerFmt{
		size:   sizeHdr,
		name:   schemaHdr,
		nameW:  nameWidthSchemas,
		extras: fmt.Sprintf(" %7s %5s %9s", "TABLES", "IDX", "ROWS"),
	}.String())
	for i := start; i < end; i++ {
		s := m.schs[i]
		pct := 0.0
		if total > 0 {
			pct = float64(s.SizeBytes) / float64(total) * 100
		}
		row := rowFmt{
			cur:    cursor(i, m.cursor),
			size:   humanize(s.SizeBytes),
			pct:    pct,
			bar:    bar(pct, barWidth),
			name:   trunc(s.Name, nameWidthSchemas),
			nameW:  nameWidthSchemas,
			extras: fmt.Sprintf(" %7d %5d %9s", s.TableCount, s.IndexCount, humanizeCount(s.RowCount)),
		}.String()
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
	nameW := max(min(m.width-fixedColsTables, nameWidthTablesMax), 1)
	n := len(m.tbls)
	start, end := m.pageWindow(n)
	var b strings.Builder
	fmtx.Fprintln(&b, headerFmt{
		size:   sizeHdr,
		name:   tableHdr,
		nameW:  nameW,
		extras: fmt.Sprintf(" %5s %9s", "IDX", "ROWS"),
	}.String())
	for i := start; i < end; i++ {
		t := m.tbls[i]
		pct := 0.0
		if total > 0 {
			pct = float64(t.TotalBytes) / float64(total) * 100
		}
		row := rowFmt{
			cur:    cursor(i, m.cursor),
			size:   humanize(t.TotalBytes),
			pct:    pct,
			bar:    bloatBar(pct, t.BloatPct, barWidth),
			name:   trunc(t.Name, nameW),
			nameW:  nameW,
			extras: fmt.Sprintf(" %5d %9s", len(t.Indexes), humanizeCount(t.RowCount)),
		}.String()
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
	fmtx.Fprintln(&b, headerFmt{
		size:   colSize,
		name:   "KIND",
		nameW:  colKindWidth,
		extras: " NAME",
	}.String())

	indexHeaderShown := false
	for i := start; i < end; i++ {
		r := m.rels[i]
		isIndex := r.Kind.IsIndex()
		if isIndex && !indexHeaderShown {
			b.WriteString(dimStyle.Render(" --- indexes " + strings.Repeat("-", indexSepWidth)))
			b.WriteString("\n")
			indexHeaderShown = true
		}

		pct := 0.0
		if total > 0 {
			pct = float64(r.SizeBytes) / float64(total) * 100
		}
		nameW := max(m.width-fixedColsRelations, 1)
		row := rowFmt{
			cur:    cursor(i, m.cursor),
			size:   humanize(r.SizeBytes),
			pct:    pct,
			bar:    bar(pct, barWidth),
			name:   r.Kind.String(),
			nameW:  colKindWidth,
			extras: " " + trunc(r.Name, nameW),
		}.String()
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
	pad := max(m.width-lipgloss.Width(left)-lipgloss.Width(right), 1)
	return left + strings.Repeat(" ", pad) + right
}

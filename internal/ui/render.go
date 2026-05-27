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

func (m *model) renderView() string {
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

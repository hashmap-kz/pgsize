package ui

import (
	"fmt"
	"sort"
	"strings"
)

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

func sortBySizeOrName[T any](s []T, size func(T) uint64, name func(T) string, bySize bool) {
	if bySize {
		sort.Slice(s, func(i, j int) bool { return size(s[i]) > size(s[j]) })
	} else {
		sort.Slice(s, func(i, j int) bool { return name(s[i]) < name(s[j]) })
	}
}

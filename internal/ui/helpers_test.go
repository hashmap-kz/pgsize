package ui

import "testing"

func TestTrunc(t *testing.T) {
	cases := []struct {
		s    string
		n    int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 4, "hel~"},
		{"hello", 3, "he~"},
		{"hello", 2, "h~"},
		{"hello", 1, "~"},
		{"hello", 0, ""},
		{"hello", -1, ""},
		{"", 5, ""},
		{"héllo", 4, "hél~"},
		{"a", 1, "a"},
		{"ab", 1, "~"},
	}
	for _, tc := range cases {
		if got := trunc(tc.s, tc.n); got != tc.want {
			t.Errorf("trunc(%q, %d) = %q, want %q", tc.s, tc.n, got, tc.want)
		}
	}
}

func TestCursor(t *testing.T) {
	if got := cursor(3, 3); got != ">" {
		t.Errorf("cursor(3,3) = %q, want %q", got, ">")
	}
	if got := cursor(2, 3); got != " " {
		t.Errorf("cursor(2,3) = %q, want %q", got, " ")
	}
	if got := cursor(0, 0); got != ">" {
		t.Errorf("cursor(0,0) = %q, want %q", got, ">")
	}
}

func TestHumanize(t *testing.T) {
	const KB = 1024
	const MB = 1024 * KB
	const GB = 1024 * MB

	cases := []struct {
		b    uint64
		want string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{1023, "1023 B"},
		{uint64(KB), "1.0 KB"},
		{uint64(KB + KB/2), "1.5 KB"},
		{uint64(2 * KB), "2.0 KB"},
		{uint64(MB), "1.0 MB"},
		{uint64(GB), "1.0 GB"},
	}
	for _, tc := range cases {
		if got := humanize(tc.b); got != tc.want {
			t.Errorf("humanize(%d) = %q, want %q", tc.b, got, tc.want)
		}
	}
}

func TestHumanizeCount(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{-1, "0"},
		{1, "~1"},
		{999, "~999"},
		{1_000, "~1.0K"},
		{1_500, "~1.5K"},
		{999_999, "~1000.0K"},
		{1_000_000, "~1.0M"},
		{12_500_000, "~12.5M"},
		{1_000_000_000, "~1.0B"},
		{5_250_000_000, "~5.2B"},
	}
	for _, tc := range cases {
		if got := humanizeCount(tc.n); got != tc.want {
			t.Errorf("humanizeCount(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestBar(t *testing.T) {
	cases := []struct {
		pct   float64
		width int
		want  string
	}{
		{0, 4, "[    ]"},
		{100, 4, "[####]"},
		{50, 4, "[##  ]"},
		{25, 4, "[#   ]"},
		{75, 4, "[### ]"},
		{0, 0, "[]"},
		{150, 4, "[####]"}, // clamped to width
		{-10, 4, "[    ]"}, // clamped to 0
	}
	for _, tc := range cases {
		if got := bar(tc.pct, tc.width); got != tc.want {
			t.Errorf("bar(%v, %d) = %q, want %q", tc.pct, tc.width, got, tc.want)
		}
	}
}

func TestBloatBar(t *testing.T) {
	cases := []struct {
		pct      float64
		bloatPct float64
		width    int
		want     string
	}{
		{100, 0, 4, "[####]"},   // no bloat: identical to bar()
		{100, 50, 4, "[##!!]"},  // 50% bloat: half live, half dead
		{100, 100, 4, "[!!!!]"}, // 100% bloat: all dead
		{50, 0, 4, "[##  ]"},    // no bloat, half full
		{50, 50, 4, "[#!  ]"},   // half full, half of that is bloat
		{0, 0, 4, "[    ]"},     // empty table
		{0, 75, 4, "[    ]"},    // no size, bloat irrelevant
		{100, 0, 0, "[]"},       // zero width
		{150, 50, 4, "[##!!]"},  // pct clamped to width
	}
	for _, tc := range cases {
		if got := bloatBar(tc.pct, tc.bloatPct, tc.width); got != tc.want {
			t.Errorf("bloatBar(%v, %v, %d) = %q, want %q",
				tc.pct, tc.bloatPct, tc.width, got, tc.want)
		}
	}
}

func TestSortBySizeOrName(t *testing.T) {
	type item struct {
		name string
		size uint64
	}
	items := []item{{"b", 100}, {"a", 300}, {"c", 200}}

	sortBySizeOrName(items,
		func(x item) uint64 { return x.size },
		func(x item) string { return x.name },
		true,
	)
	if items[0].name != "a" || items[1].name != "c" || items[2].name != "b" {
		t.Errorf("by size desc: got %v", items)
	}

	sortBySizeOrName(items,
		func(x item) uint64 { return x.size },
		func(x item) string { return x.name },
		false,
	)
	if items[0].name != "a" || items[1].name != "b" || items[2].name != "c" {
		t.Errorf("by name asc: got %v", items)
	}
}

package ui

import (
	"testing"

	"github.com/hashmap-kz/pgsize/internal/pg"
)

func TestBreadcrumb(t *testing.T) {
	cases := []struct {
		parts []string
		want  string
	}{
		{[]string{"a", "b", "c"}, "a › b › c"},
		{[]string{"only"}, "only"},
		{[]string{}, ""},
	}
	for _, tc := range cases {
		if got := breadcrumb(tc.parts...); got != tc.want {
			t.Errorf("breadcrumb(%v) = %q, want %q", tc.parts, got, tc.want)
		}
	}
}

func TestModelPageWindow(t *testing.T) {
	dbs := make([]pg.Database, 20)
	for i := range dbs {
		dbs[i] = pg.Database{Name: "db"}
	}
	m := newTestModel(viewDatabases, dbs, nil, nil, nil)
	m.height = 10 // maxRows = 10 - 6 = 4

	// cursor at 0: window [0, 4)
	m.cursor = 0
	start, end := m.pageWindow(20)
	if start != 0 || end != 4 {
		t.Errorf("pageWindow cursor=0 = (%d, %d), want (0, 4)", start, end)
	}

	// cursor at 3 (last in first page): window [0, 4)
	m.cursor = 3
	start, end = m.pageWindow(20)
	if start != 0 || end != 4 {
		t.Errorf("pageWindow cursor=3 = (%d, %d), want (0, 4)", start, end)
	}

	// cursor at 4 (just beyond first page): start=1, end=5
	m.cursor = 4
	start, end = m.pageWindow(20)
	if start != 1 || end != 5 {
		t.Errorf("pageWindow cursor=4 = (%d, %d), want (1, 5)", start, end)
	}

	// cursor near end: end clamped to n
	m.cursor = 19
	_, end = m.pageWindow(20)
	if end != 20 {
		t.Errorf("pageWindow cursor=19 end = %d, want 20 (clamped)", end)
	}
}

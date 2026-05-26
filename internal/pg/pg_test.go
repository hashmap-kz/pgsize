package pg

import "testing"

func TestRelKindString(t *testing.T) {
	cases := []struct {
		kind RelKind
		want string
	}{
		{RelUnknown, "UNKNOWN"},
		{RelHeap, "HEAP"},
		{RelToast, "TOAST"},
		{RelFsmVM, "FSM/VM"},
		{RelBtree, "BTREE"},
		{RelGin, "GIN"},
		{RelGist, "GIST"},
		{RelHash, "HASH"},
		{RelBrin, "BRIN"},
		{RelSpgist, "SPGIST"},
		{RelKind(99), "UNKNOWN"},
	}
	for _, tc := range cases {
		if got := tc.kind.String(); got != tc.want {
			t.Errorf("RelKind(%d).String() = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

func TestRelKindIsIndex(t *testing.T) {
	notIndex := []RelKind{RelHeap, RelToast, RelFsmVM}
	for _, k := range notIndex {
		if k.IsIndex() {
			t.Errorf("RelKind(%d).IsIndex() = true, want false", k)
		}
	}

	isIndex := []RelKind{RelBtree, RelGin, RelGist, RelHash, RelBrin, RelSpgist, RelUnknown, RelKind(99)}
	for _, k := range isIndex {
		if !k.IsIndex() {
			t.Errorf("RelKind(%d).IsIndex() = false, want true", k)
		}
	}
}

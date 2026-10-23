package sharecode

import (
	"regexp"
	"testing"
)

// The rule from share-api.md, kept here to pin Valid to it.
var spec = regexp.MustCompile(`^[0-9A-Za-z]{16}$`)

func TestValid(t *testing.T) {
	for _, tc := range []struct {
		code string
		ok   bool
	}{
		{"Xk3pQ9aB7mLzT2wR", true},
		{"xk3pq9ab7mlzt2wr", true},
		{"XK3PQ9AB7MLZT2WR", true},
		{"0123456789abcdef", true},
		{"Xk3pQ9aB7mLzT2w", false},   // 15
		{"Xk3pQ9aB7mLzT2wRx", false}, // 17
		{"", false},
		{"Xk3pQ9aB7mLzT2-R", false},
		{"Xk3pQ9aB7mLzT2_R", false},
		{"Xk3pQ9aB7mLzT2%R", false},
		{"Xk3pQ9aB7mLz 2wR", false},
		{"Xk3pQ9aB7mLzT/wR", false},
		{"Xk3pQ9aB7mLzT2wЖ", false}, // Cyrillic, 17 bytes
		{"Xk3pQ9aB7mLzT2Ж", false},  // Cyrillic, 16 bytes
		{"Xk3pQ9aB7mLz😀", false},    // emoji, 16 bytes
		{"Xk3pQ9aB7mL%2FwR", false},
	} {
		if got := Valid(tc.code); got != tc.ok {
			t.Errorf("Valid(%q) = %v, want %v", tc.code, got, tc.ok)
		}
		if got := spec.MatchString(tc.code); got != tc.ok {
			t.Errorf("table disagrees with spec for %q", tc.code)
		}
	}
}

func FuzzValidMatchesSpec(f *testing.F) {
	f.Add("Xk3pQ9aB7mLzT2wR")
	f.Add("Xk3pQ9aB7mLzT2w\x00")
	f.Fuzz(func(t *testing.T, s string) {
		if Valid(s) != spec.MatchString(s) {
			t.Fatalf("Valid(%q) disagrees with spec", s)
		}
	})
}

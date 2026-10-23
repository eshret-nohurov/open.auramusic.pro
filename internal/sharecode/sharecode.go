// Package sharecode knows what a share code looks like. The apps apply the
// same rule before asking the backend, so the site rejects exactly what
// they reject.
package sharecode

const Len = 16

// Valid reports whether s is exactly 16 ASCII letters or digits. Case
// matters: codes differing only in case are different codes.
func Valid(s string) bool {
	if len(s) != Len {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('0' <= c && c <= '9' || 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z') {
			return false
		}
	}
	return true
}

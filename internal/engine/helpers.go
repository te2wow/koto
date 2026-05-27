package engine

import "strings"

// containsMarker reports whether the agent output contains the transition marker.
// Matching is whitespace-insensitive on the marker's own surrounding space so a
// marker like "__NEXT:test__" is found regardless of formatting around it.
func containsMarker(output, marker string) bool {
	m := strings.TrimSpace(marker)
	if m == "" {
		return false
	}
	return strings.Contains(output, m)
}

// truncate shortens s to at most n runes, appending an ellipsis when cut.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

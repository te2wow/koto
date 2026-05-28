package engine

import (
	"strings"

	"github.com/te2wow/koto/internal/workflow"
)

// markerReminder builds a system-prompt instruction telling the agent it MUST end
// its reply with exactly one of the step's transition markers. This is appended
// out-of-band so the agent reliably emits a marker even if a host config nudges
// it toward other behavior — without it, a forgotten marker stalls the workflow.
func markerReminder(rules []workflow.Rule) string {
	var markers []string
	for _, r := range rules {
		m := strings.TrimSpace(r.On)
		if m != "" {
			markers = append(markers, m)
		}
	}
	if len(markers) == 0 {
		return ""
	}
	return "IMPORTANT (koto workflow control): your reply MUST end with exactly one " +
		"of these markers on its own final line, and nothing after it: " +
		strings.Join(markers, " , ") + ". Do not perform any unrelated side tasks."
}

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

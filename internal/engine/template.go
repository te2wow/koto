package engine

import (
	"regexp"
	"strings"
)

// templateVar matches {{ name }} and {{ vars.name }} placeholders.
var templateVar = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}`)

// render substitutes template variables in s using the provided values.
// Unknown variables are replaced with an empty string. Supports dotted
// "vars.<key>" lookups via the vars map.
func render(s string, ctx renderContext) string {
	return templateVar.ReplaceAllStringFunc(s, func(m string) string {
		key := strings.TrimSpace(templateVar.FindStringSubmatch(m)[1])
		if name, ok := strings.CutPrefix(key, "vars."); ok {
			return ctx.Vars[name]
		}
		if v, ok := ctx.Scalars[key]; ok {
			return v
		}
		return ""
	})
}

// renderContext carries the values available to template rendering.
type renderContext struct {
	Vars    map[string]string
	Scalars map[string]string // task, prev, gate_output, iteration
}

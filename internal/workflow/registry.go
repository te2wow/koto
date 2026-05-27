package workflow

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed builtins/*.yaml
var builtinFS embed.FS

// Source describes where a workflow definition came from.
type Source string

const (
	SourceLocal   Source = "local"   // ./.koto/workflows
	SourceUser    Source = "user"    // ~/.koto/workflows
	SourceBuiltin Source = "builtin" // embedded
)

// Entry is a discovered workflow available for use.
type Entry struct {
	Name   string
	Source Source
	Path   string // filesystem path, empty for builtin
}

// localDir and userDir are the on-disk workflow search locations.
func localDir() string { return filepath.Join(".koto", "workflows") }

func userDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".koto", "workflows")
}

// Resolve finds a workflow by name, honoring precedence:
// local (.koto/workflows) → user (~/.koto/workflows) → builtin.
func Resolve(name string) (*Workflow, Source, error) {
	for _, dir := range []struct {
		path string
		src  Source
	}{
		{localDir(), SourceLocal},
		{userDir(), SourceUser},
	} {
		if dir.path == "" {
			continue
		}
		p := filepath.Join(dir.path, name+".yaml")
		if _, err := os.Stat(p); err == nil {
			wf, err := Load(p)
			if err != nil {
				return nil, dir.src, err
			}
			return wf, dir.src, nil
		}
	}

	// Builtin fallback.
	data, err := builtinFS.ReadFile("builtins/" + name + ".yaml")
	if err != nil {
		return nil, "", fmt.Errorf("workflow %q not found (looked in %s, %s, and builtins)", name, localDir(), userDir())
	}
	wf, perr := Parse(data)
	if perr != nil {
		return nil, SourceBuiltin, perr
	}
	return wf, SourceBuiltin, nil
}

// List returns all available workflows across sources, de-duplicated by the
// precedence rule (an earlier source shadows later ones with the same name).
func List() []Entry {
	seen := map[string]bool{}
	var entries []Entry

	add := func(name string, src Source, path string) {
		if seen[name] {
			return
		}
		seen[name] = true
		entries = append(entries, Entry{Name: name, Source: src, Path: path})
	}

	for _, dir := range []struct {
		path string
		src  Source
	}{
		{localDir(), SourceLocal},
		{userDir(), SourceUser},
	} {
		if dir.path == "" {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(dir.path, "*.yaml"))
		sort.Strings(matches)
		for _, m := range matches {
			name := strings.TrimSuffix(filepath.Base(m), ".yaml")
			add(name, dir.src, m)
		}
	}

	_ = fs.WalkDir(builtinFS, "builtins", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".yaml") {
			return nil
		}
		name := strings.TrimSuffix(filepath.Base(p), ".yaml")
		add(name, SourceBuiltin, "")
		return nil
	})

	return entries
}

// BuiltinBytes returns the raw bytes of a builtin workflow (for `koto init`).
func BuiltinBytes(name string) ([]byte, error) {
	return builtinFS.ReadFile("builtins/" + name + ".yaml")
}

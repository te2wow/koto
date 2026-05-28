package dashboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/te2wow/koto/internal/workflow"
)

// workflowsList: GET /api/workflows → all known workflows across sources.
func (s *Server) handleWorkflowsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	entries := workflow.List()
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]any{
			"name":   e.Name,
			"source": string(e.Source),
			"path":   e.Path,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleWorkflowItem routes /api/workflows/{scope}/{name}[/action].
// Methods: GET (read), PUT (save), DELETE (remove), POST /duplicate, POST /move.
func (s *Server) handleWorkflowItem(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/workflows/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 2 {
		http.Error(w, "expected /api/workflows/{scope}/{name}", http.StatusBadRequest)
		return
	}
	scope := workflow.Source(parts[0])
	name := parts[1]
	action := ""
	if len(parts) >= 3 {
		action = parts[2]
	}
	if !validName(name) {
		http.Error(w, "invalid workflow name", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.readWorkflow(w, scope, name)
	case http.MethodPut:
		s.writeWorkflow(w, r, scope, name)
	case http.MethodDelete:
		s.deleteWorkflow(w, scope, name)
	case http.MethodPost:
		switch action {
		case "duplicate":
			s.duplicateWorkflow(w, r, scope, name)
		case "move":
			s.moveWorkflow(w, r, scope, name)
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) readWorkflow(w http.ResponseWriter, scope workflow.Source, name string) {
	data, err := workflow.ReadRaw(scope, name)
	if err != nil {
		statusForWorkflowErr(w, err)
		return
	}
	p := ""
	if scope != workflow.SourceBuiltin {
		if pp, err := pathOf(scope, name); err == nil {
			p = pp
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"scope":    string(scope),
		"name":     name,
		"path":     p,
		"yaml":     string(data),
		"readOnly": scope == workflow.SourceBuiltin,
	})
}

func (s *Server) writeWorkflow(w http.ResponseWriter, r *http.Request, scope workflow.Source, name string) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512*1024))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req struct {
		Yaml string `json:"yaml"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "expected JSON {yaml: ...}", http.StatusBadRequest)
		return
	}
	if err := workflow.WriteRaw(scope, name, []byte(req.Yaml)); err != nil {
		statusForWorkflowErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) deleteWorkflow(w http.ResponseWriter, scope workflow.Source, name string) {
	if err := workflow.Delete(scope, name); err != nil {
		statusForWorkflowErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) duplicateWorkflow(w http.ResponseWriter, r *http.Request, scope workflow.Source, name string) {
	var req struct {
		ToScope workflow.Source `json:"toScope"`
		ToName  string          `json:"toName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !validName(req.ToName) {
		http.Error(w, "expected {toScope, toName}", http.StatusBadRequest)
		return
	}
	if req.ToScope == "" {
		req.ToScope = scope
		if scope == workflow.SourceBuiltin {
			req.ToScope = workflow.SourceLocal
		}
	}
	if err := workflow.Copy(scope, name, req.ToScope, req.ToName); err != nil {
		statusForWorkflowErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "scope": string(req.ToScope), "name": req.ToName})
}

func (s *Server) moveWorkflow(w http.ResponseWriter, r *http.Request, scope workflow.Source, name string) {
	var req struct {
		ToScope workflow.Source `json:"toScope"`
		ToName  string          `json:"toName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "expected {toScope, toName}", http.StatusBadRequest)
		return
	}
	if req.ToName == "" {
		req.ToName = name
	}
	if !validName(req.ToName) {
		http.Error(w, "invalid destination name", http.StatusBadRequest)
		return
	}
	if err := workflow.Move(scope, name, req.ToScope, req.ToName); err != nil {
		statusForWorkflowErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "scope": string(req.ToScope), "name": req.ToName})
}

// handleValidate: POST /api/validate with body {"yaml": "..."} → ok/errors.
func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, _ := io.ReadAll(http.MaxBytesReader(w, r.Body, 512*1024))
	var req struct {
		Yaml string `json:"yaml"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "expected JSON {yaml: ...}"})
		return
	}
	wf, err := workflow.Parse([]byte(req.Yaml))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"name":    wf.Name,
		"initial": wf.Initial,
		"steps":   len(wf.Steps),
	})
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func validName(s string) bool { return s != "" && nameRe.MatchString(s) }

// pathOf is a thin re-export so the API package can show absolute paths.
func pathOf(scope workflow.Source, name string) (string, error) {
	dir := ""
	switch scope {
	case workflow.SourceLocal:
		dir = filepath.Join(".koto", "workflows")
	case workflow.SourceUser:
		dir = workflowsUserDir()
	}
	if dir == "" {
		return "", errors.New("no path for scope")
	}
	return filepath.Join(dir, name+".yaml"), nil
}

func workflowsUserDir() string {
	// Mirror workflow.userDir() without importing internals.
	for _, e := range workflow.List() {
		if e.Source == workflow.SourceUser && e.Path != "" {
			return filepath.Dir(e.Path)
		}
	}
	return ""
}

// statusForWorkflowErr maps known workflow-package errors to HTTP statuses.
func statusForWorkflowErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, workflow.ErrReadOnly):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, workflow.ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, fmt.Sprint(err), http.StatusBadRequest)
	}
}

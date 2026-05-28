package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// newTestServer wires a Server against a temp rootDir, so tests can stage
// .koto/runs/ contents on disk.
func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := New("127.0.0.1:0", dir, false)
	if err != nil {
		t.Fatalf("server new: %v", err)
	}
	return s, dir
}

// writeRun creates a fake run directory with events.jsonl content.
func writeRun(t *testing.T, root, id, content string) {
	t.Helper()
	d := filepath.Join(root, ".koto", "runs", id)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(d, "events.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatalf("write events: %v", err)
	}
}

func TestRunsListEmpty(t *testing.T) {
	s, _ := newTestServer(t)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/runs", nil))
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	var got []RunSummary
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %d", len(got))
	}
}

func TestRunsListSummarizes(t *testing.T) {
	s, root := newTestServer(t)
	writeRun(t, root, "20260528-100000", `{"type":"run_start","message":"wf1","detail":{"task":"do thing"},"time":"2026-05-28T10:00:00Z"}
{"type":"step_enter","step":"a"}
{"type":"step_enter","step":"b"}
{"type":"run_end","message":"complete","time":"2026-05-28T10:00:10Z"}
`)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/runs", nil))
	var got []RunSummary
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got) != 1 || got[0].Outcome != "complete" || got[0].Steps != 2 || got[0].Workflow != "wf1" {
		t.Fatalf("unexpected summary: %+v", got)
	}
}

func TestRunDetail(t *testing.T) {
	s, root := newTestServer(t)
	writeRun(t, root, "20260528-100000", `{"type":"run_start","message":"wf"}
{"type":"run_end","message":"complete"}
`)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/runs/20260528-100000", nil))
	if w.Code != 200 {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["id"] != "20260528-100000" {
		t.Fatalf("wrong id: %v", got["id"])
	}
}

func TestStats(t *testing.T) {
	s, root := newTestServer(t)
	writeRun(t, root, "20260528-100000",
		`{"type":"run_start","message":"wf"}`+"\n"+
			`{"type":"step_enter","step":"a"}`+"\n"+
			`{"type":"gate_result","step":"g"}`+"\n"+
			`{"type":"run_end","message":"complete"}`+"\n")
	writeRun(t, root, "20260528-110000",
		`{"type":"run_start","message":"wf"}`+"\n"+
			`{"type":"step_enter","step":"a"}`+"\n"+
			`{"type":"gate_result","step":"g"}`+"\n"+
			`{"type":"gate_result","step":"g"}`+"\n"+
			`{"type":"run_end","message":"abort"}`+"\n")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/stats", nil))
	var stats Stats
	_ = json.Unmarshal(w.Body.Bytes(), &stats)
	if stats.Total != 2 || stats.Completed != 1 || stats.Aborted != 1 {
		t.Fatalf("counts wrong: %+v", stats)
	}
	if stats.GateAttempt["g"] != 3 {
		t.Fatalf("gate attempts wrong: %v", stats.GateAttempt)
	}
}

func TestSplitLines(t *testing.T) {
	got := splitLines([]byte("a\nb\n"))
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("got %v", got)
	}
	got = splitLines([]byte("only"))
	if len(got) != 1 || got[0] != "only" {
		t.Fatalf("got %v", got)
	}
	if splitLines(nil) != nil {
		t.Fatal("expected nil")
	}
}

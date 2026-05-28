package dashboard

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/te2wow/koto/internal/runlog"
)

// RunSummary is the lightweight metadata returned for the runs list.
type RunSummary struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Workflow string `json:"workflow,omitempty"`
	Task     string `json:"task,omitempty"`
	Outcome  string `json:"outcome,omitempty"` // complete | abort | maxsteps | running
	Steps    int    `json:"steps"`
	Started  string `json:"started,omitempty"`
	Ended    string `json:"ended,omitempty"`
}

// handleRuns: GET /api/runs → newest-first list of runs.
func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	base := runlog.RunsBaseDir(s.rootD)
	entries, _ := os.ReadDir(base)
	runs := make([]RunSummary, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		runs = append(runs, summarize(filepath.Join(base, e.Name())))
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].ID > runs[j].ID })
	writeJSON(w, http.StatusOK, runs)
}

// handleRunDetail dispatches /api/runs/{id} and /api/runs/{id}/stream.
func (s *Server) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	id := parts[0]
	if len(parts) == 2 && parts[1] == "stream" {
		s.streamRun(w, r, id)
		return
	}
	s.getRun(w, r, id)
}

// getRun returns the full event log of a run.
func (s *Server) getRun(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	dir := filepath.Join(runlog.RunsBaseDir(s.rootD), id)
	events, err := readEvents(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      id,
		"path":    dir,
		"events":  events,
		"summary": summarize(dir),
	})
}

// streamRun pushes new events as Server-Sent Events as they are appended.
func (s *Server) streamRun(w http.ResponseWriter, r *http.Request, id string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	path := filepath.Join(runlog.RunsBaseDir(s.rootD), id, "events.jsonl")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Tail the file: emit existing lines, then poll for new lines until the client
	// disconnects or the run ends (run_end event seen).
	endSeen := false
	tailEvents(ctx, path, func(raw string) bool {
		fmt.Fprintf(w, "data: %s\n\n", raw)
		flusher.Flush()
		if strings.Contains(raw, `"type":"run_end"`) {
			endSeen = true
			return true
		}
		return false
	})
	if endSeen {
		// HTTP 204 in the SSE stream tells the client we are done; combined with
		// the client closing on run_end this prevents reconnect-replay.
		fmt.Fprintf(w, "event: close\ndata: end\n\n")
		flusher.Flush()
	}
}

// readEvents parses every JSON-line in path into a slice of generic objects.
func readEvents(path string) ([]map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	out := []map[string]any{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e map[string]any
		if err := json.Unmarshal(line, &e); err == nil {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

// summarize extracts the headline metadata from a run's events.jsonl.
func summarize(dir string) RunSummary {
	id := filepath.Base(dir)
	events, _ := readEvents(filepath.Join(dir, "events.jsonl"))
	sum := RunSummary{ID: id, Path: dir, Outcome: "running"}
	for _, e := range events {
		switch e["type"] {
		case "run_start":
			sum.Workflow, _ = e["message"].(string)
			if d, ok := e["detail"].(map[string]any); ok {
				if t, ok := d["task"].(string); ok {
					sum.Task = t
				}
			}
			if t, ok := e["time"].(string); ok {
				sum.Started = t
			}
		case "step_enter":
			sum.Steps++
		case "run_end":
			if m, ok := e["message"].(string); ok {
				sum.Outcome = m
			}
			if t, ok := e["time"].(string); ok {
				sum.Ended = t
			}
		}
	}
	return sum
}

// tailEvents emits each existing line, then watches for appended lines. The
// emit callback returning true ends the stream (used for run_end detection).
func tailEvents(ctx context.Context, path string, emit func(line string) bool) {
	var offset int64
	const pollInterval = 250 * time.Millisecond

	flush := func() bool {
		f, err := os.Open(path)
		if err != nil {
			return false
		}
		defer func() { _ = f.Close() }()
		st, err := f.Stat()
		if err != nil {
			return false
		}
		if st.Size() < offset {
			offset = 0 // file truncated/rotated
		}
		if _, err := f.Seek(offset, 0); err != nil {
			return false
		}
		// Read raw bytes so we know exactly how many we consumed (a Scanner's
		// internal buffer would overshoot the file pointer and we'd re-read
		// existing lines forever on the next poll).
		buf := make([]byte, st.Size()-offset)
		n, _ := f.Read(buf)
		offset += int64(n)
		stop := false
		for _, line := range splitLines(buf[:n]) {
			if line == "" {
				continue
			}
			if emit(line) {
				stop = true
			}
		}
		return stop
	}

	if flush() {
		return
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if flush() {
				return
			}
		}
	}
}

// splitLines breaks buf into lines without trailing newlines. Unlike bufio it
// doesn't buffer beyond the slice so caller-controlled offsets stay accurate.
func splitLines(buf []byte) []string {
	if len(buf) == 0 {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i < len(buf); i++ {
		if buf[i] == '\n' {
			out = append(out, string(buf[start:i]))
			start = i + 1
		}
	}
	if start < len(buf) {
		out = append(out, string(buf[start:]))
	}
	return out
}

// writeJSON is a tiny JSON response helper used across handlers.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

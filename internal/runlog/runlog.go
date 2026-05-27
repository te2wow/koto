// Package runlog writes a structured, append-only record of a workflow run
// under .koto/runs/<id>/ so any execution can be reconstructed afterwards.
package runlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// EventType classifies a run-log entry.
type EventType string

const (
	EventStart      EventType = "run_start"
	EventStep       EventType = "step_enter"
	EventAgent      EventType = "agent_output"
	EventGate       EventType = "gate_result"
	EventTransition EventType = "transition"
	EventApprove    EventType = "approval"
	EventEnd        EventType = "run_end"
)

// Event is a single structured log entry (one JSON line in events.jsonl).
type Event struct {
	Time    time.Time `json:"time"`
	Type    EventType `json:"type"`
	Step    string    `json:"step,omitempty"`
	Message string    `json:"message,omitempty"`
	Detail  any       `json:"detail,omitempty"`
}

// Logger appends events for a single run to disk.
type Logger struct {
	ID  string
	Dir string
	f   *os.File
}

// New creates a run directory under baseDir/.koto/runs/<id> and opens the log.
func New(baseDir string) (*Logger, error) {
	id := time.Now().Format("20060102-150405")
	dir := filepath.Join(baseDir, ".koto", "runs", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create run dir: %w", err)
	}
	f, err := os.Create(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("create run log: %w", err)
	}
	return &Logger{ID: id, Dir: dir, f: f}, nil
}

// Log appends an event. Failures to write are returned but non-fatal to callers.
func (l *Logger) Log(e Event) error {
	if l == nil || l.f == nil {
		return nil
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = l.f.Write(append(b, '\n'))
	return err
}

// Close flushes and closes the log file.
func (l *Logger) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	return l.f.Close()
}

// RunsBaseDir is the directory holding all runs for `koto list`.
func RunsBaseDir(baseDir string) string {
	return filepath.Join(baseDir, ".koto", "runs")
}

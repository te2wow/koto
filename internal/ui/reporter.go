// Package ui implements clig.dev-friendly reporters: a human-readable reporter
// for TTYs and a JSON-lines reporter for machines/agents. Progress and logs go
// to stderr; structured results go to stdout.
package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/te2wow/koto/internal/engine"
	"github.com/te2wow/koto/internal/workflow"
)

// color codes, disabled when NO_COLOR is set or output is not a TTY.
type palette struct{ reset, dim, green, red, cyan, bold string }

func newPalette(enabled bool) palette {
	if !enabled {
		return palette{}
	}
	return palette{
		reset: "\033[0m", dim: "\033[2m", green: "\033[32m",
		red: "\033[31m", cyan: "\033[36m", bold: "\033[1m",
	}
}

// Human prints readable progress to an io.Writer (typically stderr).
type Human struct {
	w io.Writer
	p palette
}

// NewHuman builds a human reporter. color is honored only when the writer is a TTY.
func NewHuman(w io.Writer, color bool) *Human {
	return &Human{w: w, p: newPalette(color && !noColorEnv())}
}

func (h *Human) StepEnter(step string, kind workflow.StepType) {
	fmt.Fprintf(h.w, "%s▶ %s%s %s(%s)%s\n", h.p.bold, step, h.p.reset, h.p.dim, kind, h.p.reset)
}

func (h *Human) AgentDone(step, output string) {
	fmt.Fprintf(h.w, "  %s· agent finished%s\n", h.p.dim, h.p.reset)
}

func (h *Human) GateResult(step string, passed bool, exitCode, attempt, maxRetries int) {
	if passed {
		fmt.Fprintf(h.w, "  %s✓ gate passed%s\n", h.p.green, h.p.reset)
		return
	}
	fmt.Fprintf(h.w, "  %s✗ gate failed (exit %d, attempt %d/%d)%s\n",
		h.p.red, exitCode, attempt, maxRetries+1, h.p.reset)
}

func (h *Human) Transition(from, to string) {
	switch to {
	case workflow.TargetComplete:
		fmt.Fprintf(h.w, "%s✓ complete%s\n", h.p.green+h.p.bold, h.p.reset)
	case workflow.TargetAbort:
		fmt.Fprintf(h.w, "%s✗ aborted%s\n", h.p.red+h.p.bold, h.p.reset)
	default:
		fmt.Fprintf(h.w, "  %s→ %s%s\n", h.p.cyan, to, h.p.reset)
	}
}

func (h *Human) Info(msg string) {
	fmt.Fprintf(h.w, "  %s%s%s\n", h.p.dim, msg, h.p.reset)
}

// JSON emits one JSON object per event to an io.Writer (typically stderr).
type JSON struct {
	enc *json.Encoder
}

// NewJSON builds a JSON-lines reporter.
func NewJSON(w io.Writer) *JSON { return &JSON{enc: json.NewEncoder(w)} }

func (j *JSON) emit(event string, fields map[string]any) {
	fields["event"] = event
	_ = j.enc.Encode(fields)
}

func (j *JSON) StepEnter(step string, kind workflow.StepType) {
	j.emit("step_enter", map[string]any{"step": step, "kind": string(kind)})
}
func (j *JSON) AgentDone(step, output string) {
	j.emit("agent_done", map[string]any{"step": step, "output": output})
}
func (j *JSON) GateResult(step string, passed bool, exitCode, attempt, maxRetries int) {
	j.emit("gate_result", map[string]any{
		"step": step, "passed": passed, "exit_code": exitCode,
		"attempt": attempt, "max_retries": maxRetries,
	})
}
func (j *JSON) Transition(from, to string) {
	j.emit("transition", map[string]any{"from": from, "to": to})
}
func (j *JSON) Info(msg string) { j.emit("info", map[string]any{"message": msg}) }

// compile-time checks that both reporters satisfy engine.Reporter.
var (
	_ engine.Reporter = (*Human)(nil)
	_ engine.Reporter = (*JSON)(nil)
)

func noColorEnv() bool {
	return strings.TrimSpace(os.Getenv("NO_COLOR")) != "" || os.Getenv("TERM") == "dumb"
}

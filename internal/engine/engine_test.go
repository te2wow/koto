package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/te2wow/koto/internal/provider"
	"github.com/te2wow/koto/internal/runlog"
	"github.com/te2wow/koto/internal/workflow"
)

// newTestEngine builds an engine with a mock provider and a real run log in a temp dir.
func newTestEngine(t *testing.T, wf *workflow.Workflow, mock *provider.Mock) *Engine {
	t.Helper()
	dir := t.TempDir()
	logger, err := runlog.New(dir)
	if err != nil {
		t.Fatalf("runlog: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	return &Engine{
		WF:       wf,
		Provider: mock,
		WorkDir:  dir,
		Log:      logger,
	}
}

func mustParse(t *testing.T, src string) *workflow.Workflow {
	t.Helper()
	wf, err := workflow.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return wf
}

// TestHappyPath: agent advances and a passing gate completes the workflow.
func TestHappyPath(t *testing.T) {
	wf := mustParse(t, `
name: t
initial: impl
max_steps: 10
steps:
  - name: impl
    type: agent
    persona: "implement {{task}}"
    rules:
      - on: "__NEXT:gate__"
        to: gate
  - name: gate
    type: gate
    run: "exit 0"
    max_retries: 2
    on_pass: COMPLETE
    on_fail: impl
`)
	mock := &provider.Mock{Responses: []string{"done __NEXT:gate__"}, Default: "__NEXT:gate__"}
	eng := newTestEngine(t, wf, mock)

	outcome, err := eng.Run(context.Background(), "the task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != OutcomeComplete {
		t.Fatalf("expected complete, got %s", outcome)
	}
	if mock.Calls() != 1 {
		t.Fatalf("expected 1 agent call, got %d", mock.Calls())
	}
}

// TestFixLoopUntilGreen: the headline behavior. A gate that fails twice then
// passes must drive the agent through fix steps until the gate is green.
func TestFixLoopUntilGreen(t *testing.T) {
	dir := t.TempDir()
	// A gate script that counts attempts via a file and passes only on the 3rd
	// run. Written to disk so the YAML 'run' command is a simple, quote-free path.
	counter := filepath.Join(dir, "n")
	script := filepath.Join(dir, "gate.sh")
	body := "#!/bin/sh\n" +
		"n=$(cat \"" + counter + "\" 2>/dev/null || echo 0)\n" +
		"n=$((n+1)); echo $n > \"" + counter + "\"\n" +
		"if [ \"$n\" -ge 3 ]; then exit 0; fi\n" +
		"echo \"attempt $n failing\" 1>&2; exit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write gate script: %v", err)
	}

	wf := mustParse(t, `
name: t
initial: impl
max_steps: 20
vars:
  gate: "sh `+script+`"
steps:
  - name: impl
    type: agent
    persona: "implement"
    rules:
      - on: "__NEXT:gate__"
        to: gate
  - name: gate
    type: gate
    run: "{{vars.gate}}"
    max_retries: 5
    on_pass: COMPLETE
    on_fail: fix
  - name: fix
    type: agent
    persona: "fix this: {{gate_output}}"
    rules:
      - on: "__NEXT:gate__"
        to: gate
`)
	mock := &provider.Mock{Default: "__NEXT:gate__"}

	logger, _ := runlog.New(dir)
	defer func() { _ = logger.Close() }()
	eng := &Engine{WF: wf, Provider: mock, WorkDir: dir, Log: logger}

	outcome, err := eng.Run(context.Background(), "task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != OutcomeComplete {
		t.Fatalf("expected complete after fix loop, got %s", outcome)
	}
	// impl(1) + fix(2 times because gate failed twice before passing on the 3rd)
	if mock.Calls() != 3 {
		t.Fatalf("expected 3 agent calls (impl + 2 fixes), got %d", mock.Calls())
	}
	// The fix prompt must have received the gate output (external feedback).
	lastPrompt := mock.Prompts[len(mock.Prompts)-1]
	if !contains(lastPrompt, "failing") {
		t.Fatalf("fix step should receive gate output, prompt was: %q", lastPrompt)
	}
}

// TestGateRetriesExhausted: a永続的に失敗する gate は ABORT する。
func TestGateRetriesExhausted(t *testing.T) {
	wf := mustParse(t, `
name: t
initial: gate
max_steps: 20
steps:
  - name: gate
    type: gate
    run: "exit 1"
    max_retries: 2
    on_pass: COMPLETE
    on_fail: fix
  - name: fix
    type: agent
    persona: "fix"
    rules:
      - on: "__NEXT:gate__"
        to: gate
`)
	mock := &provider.Mock{Default: "__NEXT:gate__"}
	eng := newTestEngine(t, wf, mock)

	outcome, err := eng.Run(context.Background(), "task")
	if outcome != OutcomeAbort {
		t.Fatalf("expected abort, got %s (err=%v)", outcome, err)
	}
}

// TestUnmatchedMarkerErrors: agent output matching no rule is an error.
func TestUnmatchedMarkerErrors(t *testing.T) {
	wf := mustParse(t, `
name: t
initial: a
max_steps: 5
steps:
  - name: a
    type: agent
    persona: "x"
    rules:
      - on: "__NEXT:b__"
        to: COMPLETE
`)
	mock := &provider.Mock{Default: "I did something but forgot the marker"}
	eng := newTestEngine(t, wf, mock)

	_, err := eng.Run(context.Background(), "task")
	if err == nil {
		t.Fatal("expected error when no rule matches")
	}
}

// TestMaxStepsStops: a workflow that never terminates hits max_steps.
func TestMaxStepsStops(t *testing.T) {
	wf := mustParse(t, `
name: t
initial: a
max_steps: 3
steps:
  - name: a
    type: agent
    persona: "x"
    rules:
      - on: "__GO__"
        to: a
`)
	mock := &provider.Mock{Default: "__GO__"}
	eng := newTestEngine(t, wf, mock)

	outcome, _ := eng.Run(context.Background(), "task")
	if outcome != OutcomeMaxSteps {
		t.Fatalf("expected maxsteps, got %s", outcome)
	}
}

// TestDryRun: dry run never calls the provider.
func TestDryRun(t *testing.T) {
	wf := mustParse(t, `
name: t
initial: a
max_steps: 5
steps:
  - name: a
    type: agent
    persona: "x"
    rules:
      - on: "__NEXT:g__"
        to: g
  - name: g
    type: gate
    run: "exit 1"
    max_retries: 0
    on_pass: COMPLETE
    on_fail: a
`)
	mock := &provider.Mock{Default: "__NEXT:g__"}
	eng := newTestEngine(t, wf, mock)
	eng.DryRun = true

	outcome, err := eng.Run(context.Background(), "task")
	if err != nil {
		t.Fatalf("dry run error: %v", err)
	}
	if mock.Calls() != 0 {
		t.Fatalf("dry run must not call provider, got %d calls", mock.Calls())
	}
	if outcome != OutcomeComplete {
		t.Fatalf("dry run should trace to complete, got %s", outcome)
	}
}

// TestRunLogWritten: a run produces a non-empty events.jsonl.
func TestRunLogWritten(t *testing.T) {
	wf := mustParse(t, `
name: t
initial: g
max_steps: 5
steps:
  - name: g
    type: gate
    run: "exit 0"
    max_retries: 0
    on_pass: COMPLETE
    on_fail: g
`)
	dir := t.TempDir()
	logger, _ := runlog.New(dir)
	eng := &Engine{WF: wf, Provider: provider.NewMock(), WorkDir: dir, Log: logger}
	if _, err := eng.Run(context.Background(), "task"); err != nil {
		t.Fatalf("run: %v", err)
	}
	_ = logger.Close()
	data, err := os.ReadFile(filepath.Join(logger.Dir, "events.jsonl"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty run log")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

package workflow

import (
	"strings"
	"testing"
)

func TestParseValidWorkflow(t *testing.T) {
	src := `
name: t
initial: a
max_steps: 5
steps:
  - name: a
    type: agent
    persona: "do {{task}}"
    rules:
      - on: "__NEXT:g__"
        to: g
  - name: g
    type: gate
    run: "true"
    max_retries: 1
    on_pass: COMPLETE
    on_fail: a
`
	wf, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.Name != "t" || wf.Initial != "a" || len(wf.Steps) != 2 {
		t.Fatalf("parsed workflow wrong: %+v", wf)
	}
	if wf.Step("g") == nil {
		t.Fatal("Step(g) should be found")
	}
}

func TestParseDefaultsMaxSteps(t *testing.T) {
	src := `
name: t
initial: a
steps:
  - name: a
    type: agent
    persona: "x"
    rules:
      - on: "__DONE__"
        to: COMPLETE
`
	wf, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.MaxSteps != 20 {
		t.Fatalf("expected default max_steps 20, got %d", wf.MaxSteps)
	}
}

func TestValidateRejectsUnknownTarget(t *testing.T) {
	src := `
name: t
initial: a
steps:
  - name: a
    type: agent
    persona: "x"
    rules:
      - on: "__NEXT__"
        to: nowhere
`
	_, err := Parse([]byte(src))
	if err == nil || !strings.Contains(err.Error(), "unknown target") {
		t.Fatalf("expected unknown target error, got: %v", err)
	}
}

func TestValidateRejectsMissingInitial(t *testing.T) {
	src := `
name: t
initial: ghost
steps:
  - name: a
    type: agent
    persona: "x"
    rules:
      - on: "__DONE__"
        to: COMPLETE
`
	_, err := Parse([]byte(src))
	if err == nil || !strings.Contains(err.Error(), "initial step") {
		t.Fatalf("expected initial-step error, got: %v", err)
	}
}

func TestValidateRejectsGateWithoutRun(t *testing.T) {
	src := `
name: t
initial: g
steps:
  - name: g
    type: gate
    on_pass: COMPLETE
    on_fail: g
`
	_, err := Parse([]byte(src))
	if err == nil || !strings.Contains(err.Error(), "requires a 'run'") {
		t.Fatalf("expected gate run error, got: %v", err)
	}
}

func TestValidateRejectsReservedStepName(t *testing.T) {
	src := `
name: t
initial: COMPLETE
steps:
  - name: COMPLETE
    type: agent
    persona: "x"
    rules:
      - on: "x"
        to: COMPLETE
`
	_, err := Parse([]byte(src))
	if err == nil || !strings.Contains(err.Error(), "reserved target") {
		t.Fatalf("expected reserved-name error, got: %v", err)
	}
}

func TestValidateRejectsUnknownType(t *testing.T) {
	src := `
name: t
initial: a
steps:
  - name: a
    type: wat
`
	_, err := Parse([]byte(src))
	if err == nil || !strings.Contains(err.Error(), "unknown type") {
		t.Fatalf("expected unknown type error, got: %v", err)
	}
}

func TestBuiltinsAreValid(t *testing.T) {
	for _, name := range []string{"default", "fix-until-green"} {
		wf, src, err := Resolve(name)
		if err != nil {
			t.Fatalf("builtin %q failed to resolve: %v", name, err)
		}
		if src != SourceBuiltin {
			t.Fatalf("builtin %q resolved from %q, want builtin", name, src)
		}
		if err := wf.Validate(); err != nil {
			t.Fatalf("builtin %q is invalid: %v", name, err)
		}
	}
}

func TestListIncludesBuiltins(t *testing.T) {
	entries := List()
	found := map[string]bool{}
	for _, e := range entries {
		found[e.Name] = true
	}
	for _, name := range []string{"default", "fix-until-green"} {
		if !found[name] {
			t.Fatalf("List() missing builtin %q", name)
		}
	}
}

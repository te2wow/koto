package workflow

import (
	"errors"
	"fmt"
	"strings"
)

// Validate checks the workflow for structural correctness before execution.
// It rejects missing fields, unknown transition targets, duplicate step names,
// and the absence of a stopping condition.
func (w *Workflow) Validate() error {
	var errs []string

	if strings.TrimSpace(w.Name) == "" {
		errs = append(errs, "workflow: name is required")
	}
	if len(w.Steps) == 0 {
		errs = append(errs, "workflow: at least one step is required")
	}
	if strings.TrimSpace(w.Initial) == "" {
		errs = append(errs, "workflow: initial step is required")
	}
	if w.MaxSteps <= 0 {
		errs = append(errs, "workflow: max_steps must be positive")
	}

	// Step names must be unique and non-empty; collect the set of valid targets.
	seen := map[string]bool{}
	for i, s := range w.Steps {
		if strings.TrimSpace(s.Name) == "" {
			errs = append(errs, fmt.Sprintf("step[%d]: name is required", i))
			continue
		}
		if seen[s.Name] {
			errs = append(errs, fmt.Sprintf("step %q: duplicate step name", s.Name))
		}
		seen[s.Name] = true
		if isReserved(s.Name) {
			errs = append(errs, fmt.Sprintf("step %q: name collides with a reserved target", s.Name))
		}
	}

	// Initial step must exist.
	if w.Initial != "" && !seen[w.Initial] {
		errs = append(errs, fmt.Sprintf("workflow: initial step %q is not defined", w.Initial))
	}

	// Per-step validation.
	for _, s := range w.Steps {
		errs = append(errs, w.validateStep(s, seen)...)
	}

	if len(errs) > 0 {
		return errors.New("invalid workflow:\n  - " + strings.Join(errs, "\n  - "))
	}
	return nil
}

// validTarget reports whether target is a defined step or a reserved target.
func validTarget(target string, steps map[string]bool) bool {
	return isReserved(target) || steps[target]
}

func (w *Workflow) validateStep(s Step, steps map[string]bool) []string {
	var errs []string
	prefix := fmt.Sprintf("step %q", s.Name)

	switch s.Type {
	case StepAgent:
		if strings.TrimSpace(s.Persona) == "" {
			errs = append(errs, prefix+": agent step requires a persona prompt")
		}
		if len(s.Rules) == 0 {
			errs = append(errs, prefix+": agent step requires at least one rule")
		}
		for j, r := range s.Rules {
			if strings.TrimSpace(r.On) == "" {
				errs = append(errs, fmt.Sprintf("%s rule[%d]: 'on' marker is required", prefix, j))
			}
			if !validTarget(r.To, steps) {
				errs = append(errs, fmt.Sprintf("%s rule[%d]: unknown target %q", prefix, j, r.To))
			}
		}
	case StepGate:
		if strings.TrimSpace(s.Run) == "" {
			errs = append(errs, prefix+": gate step requires a 'run' command")
		}
		if !validTarget(s.OnPass, steps) {
			errs = append(errs, fmt.Sprintf("%s: on_pass target %q is unknown", prefix, s.OnPass))
		}
		// on_fail is optional only if there are no retries to route; require it for clarity.
		if strings.TrimSpace(s.OnFail) == "" {
			errs = append(errs, prefix+": gate step requires an 'on_fail' target")
		} else if !validTarget(s.OnFail, steps) {
			errs = append(errs, fmt.Sprintf("%s: on_fail target %q is unknown", prefix, s.OnFail))
		}
		if s.MaxRetries < 0 {
			errs = append(errs, prefix+": max_retries cannot be negative")
		}
	case StepApprove:
		if strings.TrimSpace(s.Prompt) == "" {
			errs = append(errs, prefix+": approve step requires a 'prompt'")
		}
		if len(s.Rules) == 0 {
			errs = append(errs, prefix+": approve step requires rules for approve/reject targets")
		}
		for j, r := range s.Rules {
			if !validTarget(r.To, steps) {
				errs = append(errs, fmt.Sprintf("%s rule[%d]: unknown target %q", prefix, j, r.To))
			}
		}
	case "":
		errs = append(errs, prefix+": type is required (agent | gate | approve)")
	default:
		errs = append(errs, fmt.Sprintf("%s: unknown type %q", prefix, s.Type))
	}
	return errs
}

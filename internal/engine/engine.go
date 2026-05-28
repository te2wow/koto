// Package engine runs a workflow as a finite state machine: it renders each
// step's prompt, dispatches to an agent provider or a quality gate, decides the
// next step from validated transitions, and loops until COMPLETE or ABORT.
package engine

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/te2wow/koto/internal/gate"
	"github.com/te2wow/koto/internal/provider"
	"github.com/te2wow/koto/internal/runlog"
	"github.com/te2wow/koto/internal/workflow"
)

// Outcome is the terminal result of a run.
type Outcome string

const (
	OutcomeComplete Outcome = "complete" // reached COMPLETE
	OutcomeAbort    Outcome = "abort"    // reached ABORT (e.g. gate retries exhausted)
	OutcomeMaxSteps Outcome = "maxsteps" // hit max_steps stopping condition
)

// ErrAbort is returned when the workflow reaches ABORT.
var ErrAbort = errors.New("workflow aborted")

// Reporter receives human-facing progress events. The CLI implements it to
// print TTY-aware or JSON output; the engine stays UI-agnostic.
type Reporter interface {
	StepEnter(step string, kind workflow.StepType)
	AgentDone(step string, output string)
	GateResult(step string, passed bool, exitCode int, attempt, maxRetries int)
	Transition(from, to string)
	Info(msg string)
}

// Approver decides approve-step outcomes (true = approved). The CLI supplies it.
type Approver func(prompt string) bool

// Engine executes a single workflow.
type Engine struct {
	WF       *workflow.Workflow
	Provider provider.Provider
	Model    string
	WorkDir  string
	Reporter Reporter
	Approver Approver
	Log      *runlog.Logger
	DryRun   bool
	Isolate  bool // pass provider isolation (e.g. claude --bare) on each call
}

// Run drives the workflow from its initial step until a terminal state.
func (e *Engine) Run(ctx context.Context, task string) (Outcome, error) {
	scalars := map[string]string{"task": task, "prev": "", "gate_output": "", "iteration": "0"}
	rc := renderContext{Vars: e.WF.Vars, Scalars: scalars}

	_ = e.Log.Log(runlog.Event{Type: runlog.EventStart, Message: e.WF.Name, Detail: map[string]any{
		"task": task, "provider": e.Provider.Name(), "max_steps": e.WF.MaxSteps,
	}})

	current := e.WF.Initial
	retries := map[string]int{} // gate name → attempts used

	for steps := 0; steps < e.WF.MaxSteps; steps++ {
		scalars["iteration"] = strconv.Itoa(steps)
		step := e.WF.Step(current)
		if step == nil {
			return OutcomeAbort, fmt.Errorf("internal: step %q not found", current)
		}

		e.report(func(r Reporter) { r.StepEnter(step.Name, step.Type) })
		_ = e.Log.Log(runlog.Event{Type: runlog.EventStep, Step: step.Name, Message: string(step.Type)})

		next, err := e.runStep(ctx, step, rc, scalars, retries)
		if err != nil {
			return OutcomeAbort, err
		}

		switch next {
		case workflow.TargetComplete:
			e.report(func(r Reporter) { r.Transition(step.Name, workflow.TargetComplete) })
			_ = e.Log.Log(runlog.Event{Type: runlog.EventEnd, Message: string(OutcomeComplete)})
			return OutcomeComplete, nil
		case workflow.TargetAbort:
			e.report(func(r Reporter) { r.Transition(step.Name, workflow.TargetAbort) })
			_ = e.Log.Log(runlog.Event{Type: runlog.EventEnd, Message: string(OutcomeAbort)})
			return OutcomeAbort, ErrAbort
		default:
			e.report(func(r Reporter) { r.Transition(step.Name, next) })
			_ = e.Log.Log(runlog.Event{Type: runlog.EventTransition, Step: step.Name, Message: next})
			current = next
		}
	}

	e.report(func(r Reporter) { r.Info("reached max_steps stopping condition") })
	_ = e.Log.Log(runlog.Event{Type: runlog.EventEnd, Message: string(OutcomeMaxSteps)})
	return OutcomeMaxSteps, nil
}

// runStep executes one step and returns the next target name.
func (e *Engine) runStep(ctx context.Context, step *workflow.Step, rc renderContext, scalars map[string]string, retries map[string]int) (string, error) {
	switch step.Type {
	case workflow.StepAgent:
		return e.runAgent(ctx, step, rc, scalars)
	case workflow.StepGate:
		return e.runGate(ctx, step, rc, scalars, retries)
	case workflow.StepApprove:
		return e.runApprove(step, rc)
	default:
		return "", fmt.Errorf("step %q: unsupported type %q", step.Name, step.Type)
	}
}

func (e *Engine) runAgent(ctx context.Context, step *workflow.Step, rc renderContext, scalars map[string]string) (string, error) {
	prompt := render(step.Persona, rc)
	if e.DryRun {
		e.report(func(r Reporter) { r.Info("[dry-run] would run agent for step " + step.Name) })
		// In dry-run, take the first rule's target so the plan can be traced.
		if len(step.Rules) > 0 {
			return step.Rules[0].To, nil
		}
		return workflow.TargetComplete, nil
	}

	out, err := e.Provider.Run(ctx, prompt, provider.Options{
		Model:    e.Model,
		WorkDir:  e.WorkDir,
		Edit:     step.Edit,
		Isolate:  e.Isolate,
		Reminder: markerReminder(step.Rules),
	})
	if err != nil {
		return "", fmt.Errorf("step %q: %w", step.Name, err)
	}
	scalars["prev"] = out
	e.report(func(r Reporter) { r.AgentDone(step.Name, out) })
	_ = e.Log.Log(runlog.Event{Type: runlog.EventAgent, Step: step.Name, Detail: map[string]any{"output": out}})

	for _, rule := range step.Rules {
		if containsMarker(out, rule.On) {
			return rule.To, nil
		}
	}
	return "", fmt.Errorf("step %q: agent output matched no rule (expected one of the markers); output:\n%s", step.Name, truncate(out, 500))
}

func (e *Engine) runGate(ctx context.Context, step *workflow.Step, rc renderContext, scalars map[string]string, retries map[string]int) (string, error) {
	command := render(step.Run, rc)
	if e.DryRun {
		e.report(func(r Reporter) { r.Info("[dry-run] would run gate: " + command) })
		return step.OnPass, nil
	}

	attempt := retries[step.Name] + 1
	res := gate.Run(ctx, command, e.WorkDir)
	e.report(func(r Reporter) { r.GateResult(step.Name, res.Passed, res.ExitCode, attempt, step.MaxRetries) })
	_ = e.Log.Log(runlog.Event{Type: runlog.EventGate, Step: step.Name, Detail: map[string]any{
		"command": command, "passed": res.Passed, "exit_code": res.ExitCode, "attempt": attempt,
	}})

	if res.Passed {
		return step.OnPass, nil
	}

	// Failed: feed output forward and decide whether retries remain.
	scalars["gate_output"] = res.Output
	scalars["prev"] = res.Output
	retries[step.Name] = attempt
	if attempt > step.MaxRetries {
		e.report(func(r Reporter) {
			r.Info(fmt.Sprintf("gate %q exhausted %d retries → ABORT", step.Name, step.MaxRetries))
		})
		return workflow.TargetAbort, nil
	}
	return step.OnFail, nil
}

func (e *Engine) runApprove(step *workflow.Step, rc renderContext) (string, error) {
	msg := render(step.Prompt, rc)
	approved := true
	if e.Approver != nil {
		approved = e.Approver(msg)
	}
	_ = e.Log.Log(runlog.Event{Type: runlog.EventApprove, Step: step.Name, Detail: map[string]any{"approved": approved}})

	marker := "__REJECT__"
	if approved {
		marker = "__APPROVE__"
	}
	for _, rule := range step.Rules {
		if rule.On == marker || (approved && rule.On == "") {
			return rule.To, nil
		}
	}
	// Fall back: first rule on approve, ABORT on reject.
	if approved && len(step.Rules) > 0 {
		return step.Rules[0].To, nil
	}
	return workflow.TargetAbort, nil
}

func (e *Engine) report(fn func(Reporter)) {
	if e.Reporter != nil {
		fn(e.Reporter)
	}
}

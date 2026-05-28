# koto — Design Document

> Run AI coding agents through a YAML workflow, and **don't stop until the gates are green.**

`koto` is a single-binary, dependency-free workflow runner for CLI coding agents
(Claude Code, Codex, Aider, Gemini CLI, etc.). You declare a workflow in YAML —
a sequence of steps with personas and transition rules — and `koto` drives the
agent through them. Its defining feature: **quality gates that are enforced by exit
code**. A gate runs a real command (`go test ./...`, `npm run lint`, `pytest`, …) and
if it fails, control loops back to a fix step and the agent is told exactly what broke.
The workflow does not complete until the gates pass.

## Why koto exists (market gaps it fills)

From a survey of the AI agent orchestration, CLI agent control, YAML workflow
engine, agent-reliability, and Go CLI OSS landscapes, four gaps stood out:

1. **The "force the process" layer is mostly npm/TypeScript.** The few tools that
   make an agent follow a declared process require Node and a dependency tree — an
   adoption tax for a developer command-line tool.
2. **Go tools in this space mostly do parallel session management.** They manage
   parallel worktrees but do not enforce a review/quality gate as part of the process.
3. **Quality gates are usually "suggested", not "executed".** Most tools present
   checklists or score diffs rather than running a real command and blocking on it.
4. **Self-reflection alone hurts accuracy** (ICLR'24): intrinsic self-correction
   without external feedback can *degrade* output. Review steps need an *external*
   verification signal (tests, lint, types, or a different model).

**koto's position:** a Go, single-binary, *gate-enforcing* runner. It pairs the
distribution model that works best for developer CLI tools (a zero-dependency
single binary) with the capability most tools lack (hard, executed quality gates).

## Design choices

| Axis | koto's choice | Why |
|---|---|---|
| Distribution | **Go single binary** (`go install` / curl) — zero deps | no Node/runtime to install; easy to drop into CI |
| Core enforcement | **External command exit code** (`go test`, `lint`, …) | deterministic; the model cannot fake a passing test |
| Prompt model | **One step = one prompt** | the whole workflow is readable in one file |
| Providers | **CLI exec only** | provider-agnostic; never broken by SDK churn |
| Deliberation | Linear loop (+ optional parallel reviewers) | keep the surface small and predictable |
| Reliability primitive | review loop **+ executed gates + external feedback injection** | anchor judgment on a signal outside the model |
| Footprint | **small, readable Go** | auditable in an afternoon |

The core philosophy — *trust the AI, but guarantee the process* — is kept minimal:
everything else is stripped, and the one thing added is a gate that actually *runs*
and *blocks*.

## Reliability features (grounded in research)

Implemented from day one (P0):

- **State-machine execution.** A workflow is a finite state machine; the next step is
  chosen from an enumerated set, validated by the runner, never left to free text.
  (FSM externalizes an immutable source of truth — arXiv 2410.18528 / 2403.11322.)
- **Executed quality gates (the headline feature).** A `gate` step runs a shell command;
  exit 0 = pass and advance, non-zero = the command's stderr/stdout is fed back to a fix
  step and the loop continues. Generalizes Aider's test-fix loop, agent-independent.
- **External feedback in review loops.** Review/fix loops are anchored on gate output
  (tests/lint/types), not on the same model second-guessing itself.
- **Stopping conditions.** Global `max_steps` and per-gate `max_retries` prevent runaway
  loops and cost. (Anthropic: stopping conditions are required for control.)
- **Step-level observability.** Every step, agent invocation, gate run, transition, and
  retry is written to a structured run log under `.koto/runs/<id>/`.
- **Context carried by the workflow.** Outputs are passed forward via `{{prev}}`,
  `{{task}}`, `{{iteration}}` template variables; the agent need not remember.

Optional / later (P1–P2):

- worktree isolation (`--isolate`), parallel reviewer steps, human approval gate
  (`approve: true` step) for irreversible actions.

## Architecture

```
koto run <task> [--workflow NAME] [--isolate] [--json] [--dry-run]
  │
  ├── config loader      ~/.koto/config.yaml  (provider, model, language)
  ├── workflow loader    resolves NAME → .koto/workflows → ~/.koto/workflows → builtins
  │                      parses & validates YAML against schema
  ├── engine (FSM)       holds run state; for each step:
  │     ├── render prompt (template vars: task, prev, iteration, gate output)
  │     ├── dispatch:
  │     │     ├── agent step → provider.Run(prompt) → captures output
  │     │     │                 parse __NEXT:x__ / __DONE__ / __ABORT__ from output
  │     │     └── gate step  → exec shell cmd; exit 0 → next; non-zero → on_fail route
  │     ├── apply transition rule (validated against declared steps)
  │     └── append to run log
  └── providers          CLI exec adapters: claude, codex, aider, gemini, copilot, mock
        provider.Run(prompt) = exec.Command(bin, args...) with prompt on argv/stdin
```

### Package layout (Go)

```
cmd/koto/main.go         CLI entrypoint (cobra)
internal/config/             config.yaml loading, XDG paths
internal/workflow/           YAML schema structs, parse, validate, builtin workflows (embed)
internal/engine/             FSM runner, template rendering, transition logic
internal/provider/           Provider interface + claude/codex/aider/gemini/mock adapters
internal/gate/               shell command execution, exit-code handling, output capture
internal/runlog/             structured per-run logging (.koto/runs/<id>/)
internal/ui/                 TTY-aware human output + --json output (clig.dev compliant)
```

## YAML workflow schema

Designed from the YAML-workflow survey: declarative backbone + intelligence per step,
first-class retry/gate, explicit control-flow fields (not buried in an expression
language), JSON-Schema-validatable, with a code escape hatch (gate steps run arbitrary
shell).

```yaml
name: implement-test-review        # workflow id
initial: plan                      # starting step
max_steps: 20                      # global stopping condition

vars:                              # optional static vars, available as {{vars.x}}
  test_cmd: "go test ./..."

steps:
  - name: plan
    type: agent                    # agent | gate | approve
    persona: |                     # inline prompt (one step = one prompt)
      You are a planner. Read the task and produce a concise implementation plan.
      Task: {{task}}
      When the plan is complete, end your message with __NEXT:implement__
    edit: false                    # informational; agent told it is read-only
    rules:
      - on: "__NEXT:implement__"    # match agent output marker
        to: implement

  - name: implement
    type: agent
    persona: |
      Implement the plan. Make the code changes directly.
      Plan: {{prev}}
      When done, end with __NEXT:test__
    edit: true
    rules:
      - on: "__NEXT:test__"
        to: test

  - name: test                     # the headline: an executed gate
    type: gate
    run: "{{vars.test_cmd}}"        # real shell command
    max_retries: 3
    on_pass: review                 # exit 0 → advance
    on_fail: fix                    # non-zero → route to fix (with output injected)

  - name: fix
    type: agent
    persona: |
      The tests failed. Fix the code so they pass. Do not edit or delete tests.
      Failure output:
      {{gate_output}}
      When done, end with __NEXT:test__   # loops back to the gate
    edit: true
    rules:
      - on: "__NEXT:test__"
        to: test

  - name: review
    type: agent
    persona: |
      Review the final diff for correctness and clarity. Task: {{task}}
      If acceptable end with __DONE__, otherwise end with __NEXT:fix__
    edit: false
    rules:
      - on: "__DONE__"
        to: COMPLETE              # reserved terminal: success
      - on: "__NEXT:fix__"
        to: fix
```

### Schema rules

- `type: agent` runs a provider; the agent's output is scanned for `rules[].on` markers
  to decide the transition. Reserved targets: `COMPLETE` (success), `ABORT` (failure).
- `type: gate` runs `run` as a shell command. Exit 0 → `on_pass`. Non-zero →
  if retries remain and `on_fail` is set, the captured output is bound to `{{gate_output}}`
  and control goes to `on_fail`; when `on_fail` loops back to the gate (directly or via a
  fix step) retries decrement; exhausting `max_retries` → `ABORT`.
- `type: approve` (P1) pauses for human approve/reject before continuing — for
  irreversible actions, per the risk-based HITL pattern.
- Template vars: `{{task}}` (original task), `{{prev}}` (previous step output),
  `{{iteration}}` (loop count), `{{gate_output}}` (last gate's output), `{{vars.x}}`.
- Validation: unknown step targets, missing `initial`, cycles with no stopping
  condition, and missing required fields are rejected before execution.

## CLI design (clig.dev + AI-agent-friendly)

- `koto run <task>` — run a workflow on a task description.
- `koto list` — list previous runs (from `.koto/runs/`).
- `koto workflows` — list available workflows (local → user → builtin).
- `koto validate <file>` — validate a workflow YAML.
- `koto init` — scaffold `.koto/` with a starter workflow.
- `koto version` — version info.

Global flags: `--json` (machine output to stdout, logs to stderr), `--dry-run`
(print the plan without calling the agent or running gates), `--no-input`
(never prompt), `--workflow NAME`, `--provider NAME`, `--isolate` (worktree).
Exit codes: 0 success, 1 generic failure, 2 usage error, 3 workflow ABORT,
4 gate exhausted retries. Honors `NO_COLOR`, detects TTY, secrets never via flags.

## Providers (CLI exec)

A provider is `Run(ctx, prompt, opts) (output string, err error)` implemented by
exec-ing the agent CLI. Built-in:

- `claude` → `claude -p <prompt>` (non-interactive print mode)
- `codex` → `codex exec <prompt>`
- `aider` → `aider --message <prompt> --yes`
- `gemini` → `gemini -p <prompt>`
- `copilot` → `copilot -p <prompt>`
- `mock` → deterministic echo provider for tests (no network)

Provider/model come from `~/.koto/config.yaml` or `--provider`. Because we only
exec CLIs, koto is never broken by provider SDK churn.

## Testing strategy

- Unit tests: workflow parse/validate, template rendering, FSM transitions, gate
  exit-code routing, retry exhaustion, run-log writing.
- `mock` provider + a gate that flips from failing to passing → exercises the full
  implement→test→fix→test→review→COMPLETE loop with zero network.
- E2E: a real workflow driven by the actual `claude` CLI on a tiny throwaway repo,
  asserting that a deliberately failing test gets fixed until green.
- CI: GitHub Actions runs `go vet`, `go test ./...`, `golangci-lint`, and builds
  cross-platform binaries.

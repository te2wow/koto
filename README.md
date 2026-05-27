# koto

> Run AI coding agents through a YAML workflow, and **don't stop until the gates are green.**

`koto` is a single-binary, dependency-free workflow runner for CLI coding agents
(Claude Code, Codex, Aider, Gemini CLI, …). You declare a workflow in YAML — a
sequence of steps with prompts and transition rules — and `koto` drives the agent
through it.

Its defining feature: **quality gates enforced by exit code.** A gate step runs a
real command (`go test ./...`, `npm run lint`, `pytest`, …). Exit 0 → advance.
Non-zero → the command's output is fed back to a fix step and the loop continues.
**The workflow does not complete until the gates pass.**

```
▶ implement (agent)   → gate
▶ gate (gate)         ✗ gate failed (exit 1, attempt 1/6)   → fix
▶ fix (agent)         → gate
▶ gate (gate)         ✓ gate passed
✓ complete
```

## Why koto?

AI coding agents are powerful, but they forget instructions, skip reviews, and
declare success too early. Adding rules to prompts doesn't *enforce* anything —
whether they're followed is left to the model.

koto moves the decision out of the agent and into the workflow, and anchors it on a
signal the model can't fake: **the exit code of a real command.** Tests either pass
or they don't.

This idea isn't new — [`takt`](https://github.com/nrslib/takt) pioneered
YAML-driven, review-enforcing agent workflows. koto takes the same philosophy
("trust the AI, but guarantee the process") and makes three deliberate choices:

| | takt | **koto** |
|---|---|---|
| Distribution | TypeScript / npm | **Go single binary, zero deps** |
| What enforces the loop | an AI reviewer's judgment | **a real command's exit code** |
| Prompt model | Faceted Prompting (5 files/step) | **one step = one prompt** |
| Providers | SDKs + CLIs | **CLI exec only** (never broken by SDK churn) |
| Footprint | full framework | **small, readable, auditable** |

See [`docs/vs-takt.html`](docs/vs-takt.html) for the full comparison.

## Install

```bash
go install github.com/te2wow/koto/cmd/koto@latest
```

Or grab a binary from [Releases](https://github.com/te2wow/koto/releases).

You also need at least one agent CLI on your `PATH` — e.g.
[Claude Code](https://claude.ai/code) (`claude`), `codex`, `aider`, or `gemini`.

## Quick start

```bash
# 1. Scaffold a workflow into .koto/
koto init

# 2. Point the gate at your test command and run a task
koto run "add an /health endpoint" \
  --workflow fix-until-green \
  --set test_cmd="go test ./..."
```

koto creates a run log under `.koto/runs/<id>/` and drives the agent through the
workflow until the gate passes or `max_steps` is hit.

## How it works

A workflow is a finite state machine. Each step is one of three types:

- **`agent`** — runs a coding agent with a prompt; its output is scanned for
  transition markers (`__NEXT:x__`, `__DONE__`) to pick the next step.
- **`gate`** — runs a shell command; exit 0 routes to `on_pass`, non-zero routes to
  `on_fail` (with the output bound to `{{gate_output}}`), until `max_retries` is
  exhausted (then `ABORT`).
- **`approve`** — pauses for human approve/reject (for irreversible actions).

Reserved targets `COMPLETE` and `ABORT` terminate the run.

### Minimal workflow

```yaml
name: fix-until-green
initial: implement
max_steps: 20
vars:
  test_cmd: "go test ./..."

steps:
  - name: implement
    type: agent
    edit: true
    persona: |
      Implement this task by editing the code:
      {{task}}
      When done, end your message with __NEXT:gate__
    rules:
      - on: "__NEXT:gate__"
        to: gate

  - name: gate
    type: gate
    run: "{{vars.test_cmd}}"
    max_retries: 5
    on_pass: COMPLETE      # exit 0 → done
    on_fail: fix           # non-zero → fix and retry

  - name: fix
    type: agent
    edit: true
    persona: |
      The gate failed. Fix the code so it passes. Do not weaken the tests.
      Gate output:
      {{gate_output}}
      When done, end with __NEXT:gate__
    rules:
      - on: "__NEXT:gate__"
        to: gate
```

### Template variables

| Variable | Meaning |
|---|---|
| `{{task}}` | the task description passed to `koto run` |
| `{{prev}}` | the previous step's output |
| `{{gate_output}}` | the last gate's combined stdout/stderr |
| `{{iteration}}` | the loop counter |
| `{{vars.NAME}}` | a workflow var (override with `--set NAME=value`) |

## Commands

```
koto run <task>          Run a workflow on a task until the gates pass
koto workflows           List available workflows (local → user → builtin)
koto validate <file>     Validate a workflow YAML
koto init                Scaffold .koto/ with a starter workflow
koto list                List previous runs
koto version             Print version
```

### `run` flags

| Flag | Default | Description |
|---|---|---|
| `--workflow, -w` | `default` | workflow name |
| `--provider, -p` | from config | `claude` `codex` `aider` `gemini` `copilot` `mock` |
| `--model, -m` | from config | model passed to the provider |
| `--set k=v` | — | override a workflow var (repeatable) |
| `--json` | off | machine-readable JSON events on stderr |
| `--dry-run` | off | trace the workflow without calling the agent |
| `--no-input` | off | never prompt; auto-approve approval steps |
| `--bare` | off | isolate the agent from the host's global config/hooks |

Exit codes: `0` success · `1` failure · `2` usage · `3` workflow ABORT.

## Configuration

`~/.koto/config.yaml` (all fields optional):

```yaml
provider: claude   # claude | codex | aider | gemini | copilot | mock
model: ""          # passed to the provider when supported
language: en
```

Workflow resolution precedence: `./.koto/workflows/` → `~/.koto/workflows/` →
built-ins (`default`, `fix-until-green`).

## Design

The full design rationale, market analysis, and reliability decisions are in
[`DESIGN.md`](DESIGN.md). koto's reliability features are grounded in published
research (FSM-based agent control, external-feedback review loops, stopping
conditions, structured observability).

## License

MIT © te2wow

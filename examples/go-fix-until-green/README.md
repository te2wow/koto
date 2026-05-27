# Example: Go fix-until-green

A workflow that implements a task, then loops `go test` → fix until tests pass,
then runs `go vet` as a final gate.

```bash
cd examples/go-fix-until-green
koto run "make Add return a+b" --workflow go-tdd
```

The `go-tdd` workflow lives in `.koto/workflows/go-tdd.yaml` and is resolved
ahead of the built-ins because it is project-local.

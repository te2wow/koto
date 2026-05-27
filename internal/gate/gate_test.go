package gate

import (
	"context"
	"strings"
	"testing"
)

func TestGatePasses(t *testing.T) {
	res := Run(context.Background(), "exit 0", "")
	if !res.Passed || res.ExitCode != 0 {
		t.Fatalf("expected pass with exit 0, got %+v", res)
	}
}

func TestGateFailsWithExitCode(t *testing.T) {
	res := Run(context.Background(), "exit 3", "")
	if res.Passed {
		t.Fatal("expected failure")
	}
	if res.ExitCode != 3 {
		t.Fatalf("expected exit code 3, got %d", res.ExitCode)
	}
}

func TestGateCapturesOutput(t *testing.T) {
	res := Run(context.Background(), "echo hello-stdout; echo hello-stderr 1>&2; exit 1", "")
	if res.Passed {
		t.Fatal("expected failure")
	}
	if !strings.Contains(res.Output, "hello-stdout") || !strings.Contains(res.Output, "hello-stderr") {
		t.Fatalf("expected combined output, got %q", res.Output)
	}
}

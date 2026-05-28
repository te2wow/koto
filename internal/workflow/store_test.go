package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// validYAML returns a minimal valid workflow under a given name.
func validYAML(name string) []byte {
	return []byte(`name: ` + name + `
initial: a
max_steps: 5
steps:
  - name: a
    type: agent
    persona: "x"
    rules:
      - on: "__DONE__"
        to: COMPLETE
`)
}

// withChdir runs fn inside a fresh tempdir as the working directory so that
// local scope (./.koto/workflows) is sandboxed per test.
func withChdir(t *testing.T, fn func()) {
	t.Helper()
	old, _ := os.Getwd()
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	fn()
}

func TestWriteThenReadLocal(t *testing.T) {
	withChdir(t, func() {
		if err := WriteRaw(SourceLocal, "foo", validYAML("foo")); err != nil {
			t.Fatalf("write: %v", err)
		}
		data, err := ReadRaw(SourceLocal, "foo")
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if len(data) == 0 {
			t.Fatal("empty")
		}
		// Resolve should also find it now and prefer local over builtin.
		wf, src, err := Resolve("foo")
		if err != nil || wf == nil || src != SourceLocal {
			t.Fatalf("resolve: %v src=%v", err, src)
		}
	})
}

func TestWriteBuiltinRejected(t *testing.T) {
	withChdir(t, func() {
		err := WriteRaw(SourceBuiltin, "x", validYAML("x"))
		if !errors.Is(err, ErrReadOnly) {
			t.Fatalf("expected ErrReadOnly, got %v", err)
		}
	})
}

func TestInvalidYAMLNotPersisted(t *testing.T) {
	withChdir(t, func() {
		err := WriteRaw(SourceLocal, "bad", []byte(`name: bad
initial: ghost
steps: []
`))
		if err == nil {
			t.Fatal("expected validation error")
		}
		p := filepath.Join(localDir(), "bad.yaml")
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("invalid workflow should not be persisted: %v", err)
		}
	})
}

func TestCopyAndDelete(t *testing.T) {
	withChdir(t, func() {
		_ = WriteRaw(SourceLocal, "src", validYAML("src"))
		if err := Copy(SourceLocal, "src", SourceLocal, "dst"); err != nil {
			t.Fatalf("copy: %v", err)
		}
		if _, err := ReadRaw(SourceLocal, "dst"); err != nil {
			t.Fatalf("read copy: %v", err)
		}
		if err := Delete(SourceLocal, "dst"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := ReadRaw(SourceLocal, "dst"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestCopyFromBuiltin(t *testing.T) {
	withChdir(t, func() {
		if err := Copy(SourceBuiltin, "default", SourceLocal, "my-default"); err != nil {
			t.Fatalf("copy builtin: %v", err)
		}
		if _, err := ReadRaw(SourceLocal, "my-default"); err != nil {
			t.Fatalf("read: %v", err)
		}
	})
}

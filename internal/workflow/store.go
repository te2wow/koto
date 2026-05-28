package workflow

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ErrReadOnly is returned when a write is attempted against a builtin workflow.
var ErrReadOnly = errors.New("builtin workflows are read-only")

// ErrNotFound is returned when a (scope, name) pair has no on-disk file.
var ErrNotFound = errors.New("workflow not found")

// scopeDir maps a Source to its filesystem directory; builtins have none.
func scopeDir(s Source) (string, error) {
	switch s {
	case SourceLocal:
		return localDir(), nil
	case SourceUser:
		d := userDir()
		if d == "" {
			return "", errors.New("user home directory is not available")
		}
		return d, nil
	case SourceBuiltin:
		return "", ErrReadOnly
	default:
		return "", fmt.Errorf("unknown scope %q", s)
	}
}

// pathOf returns the on-disk path for a (scope, name) workflow file.
func pathOf(scope Source, name string) (string, error) {
	dir, err := scopeDir(scope)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".yaml"), nil
}

// ReadRaw returns the raw YAML bytes for a workflow at (scope, name). For builtins
// it returns the embedded bytes.
func ReadRaw(scope Source, name string) ([]byte, error) {
	if scope == SourceBuiltin {
		return BuiltinBytes(name)
	}
	p, err := pathOf(scope, name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return data, nil
}

// WriteRaw saves raw YAML bytes to (scope, name). Builtin scope is rejected.
// The directory is created on demand. The bytes are validated before writing.
func WriteRaw(scope Source, name string, data []byte) error {
	if scope == SourceBuiltin {
		return ErrReadOnly
	}
	if _, err := Parse(data); err != nil {
		return err
	}
	p, err := pathOf(scope, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// Delete removes the workflow file at (scope, name). Builtin scope is rejected.
func Delete(scope Source, name string) error {
	if scope == SourceBuiltin {
		return ErrReadOnly
	}
	p, err := pathOf(scope, name)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// Copy duplicates a workflow to (dstScope, dstName). The destination must not yet
// exist. Builtin source is supported as a read; builtin destination is rejected.
func Copy(srcScope Source, srcName string, dstScope Source, dstName string) error {
	if dstScope == SourceBuiltin {
		return ErrReadOnly
	}
	data, err := ReadRaw(srcScope, srcName)
	if err != nil {
		return err
	}
	dst, err := pathOf(dstScope, dstName)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("destination %q already exists", dst)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// Move shifts a workflow between writable scopes (local ↔ user). The source must
// exist on disk (not a builtin), and the destination must not exist.
func Move(srcScope Source, srcName string, dstScope Source, dstName string) error {
	if srcScope == SourceBuiltin || dstScope == SourceBuiltin {
		return ErrReadOnly
	}
	srcPath, err := pathOf(srcScope, srcName)
	if err != nil {
		return err
	}
	if _, err := os.Stat(srcPath); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	dstPath, err := pathOf(dstScope, dstName)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dstPath); err == nil {
		return fmt.Errorf("destination %q already exists", dstPath)
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	return os.Rename(srcPath, dstPath)
}

// copyFile is a small helper used in tests and tooling (kept exported via Copy).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	_, err = io.Copy(out, in)
	return err
}

var _ = copyFile // exported through Copy/Move; keep helper available if needed

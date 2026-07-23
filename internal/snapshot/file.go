package snapshot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const MaxFileBytes int64 = 4 << 20

func ReadFile(workspaceRoot, logical string, maxBytes int64) (Lockfile, error) {
	resolved, err := safePath(workspaceRoot, logical, false)
	if err != nil {
		return Lockfile{}, err
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return Lockfile{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Lockfile{}, fmt.Errorf("%w: snapshot must be a regular non-symlink file", ErrUnsafePath)
	}
	if maxBytes <= 0 {
		maxBytes = MaxFileBytes
	}
	if info.Size() > maxBytes {
		return Lockfile{}, fmt.Errorf("%w: snapshot exceeds %d bytes", ErrInvalidSnapshot, maxBytes)
	}
	file, err := os.Open(resolved)
	if err != nil {
		return Lockfile{}, err
	}
	defer file.Close()
	return Decode(file)
}

func WriteFile(workspaceRoot, logical string, lock Lockfile) error {
	resolved, err := safePath(workspaceRoot, logical, true)
	if err != nil {
		return err
	}
	if info, statErr := os.Lstat(resolved); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("%w: existing output is not a regular file", ErrUnsafePath)
		}
		if info.Size() > MaxFileBytes {
			return fmt.Errorf("%w: refusing to inspect an oversized existing output", ErrUnsafePath)
		}
		existing, openErr := os.Open(resolved)
		if openErr != nil {
			return openErr
		}
		_, decodeErr := Decode(existing)
		closeErr := existing.Close()
		if decodeErr != nil {
			return fmt.Errorf("%w: refusing to overwrite a file that is not a valid repository snapshot", ErrUnsafePath)
		}
		if closeErr != nil {
			return closeErr
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	parent := filepath.Dir(resolved)
	temporary, err := os.CreateTemp(parent, ".agent-config-inspector-lock-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := Write(temporary, lock); err != nil {
		_ = temporary.Close()
		return err
	}
	info, err := temporary.Stat()
	if err != nil {
		_ = temporary.Close()
		return err
	}
	if info.Size() > MaxFileBytes {
		_ = temporary.Close()
		return fmt.Errorf("%w: generated snapshot exceeds %d bytes", ErrInvalidSnapshot, MaxFileBytes)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, resolved); err != nil {
		return err
	}
	committed = true
	return nil
}

func safePath(workspaceRoot, logical string, output bool) (string, error) {
	normalized := strings.ReplaceAll(logical, "\\", "/")
	if logical == "" || filepath.IsAbs(logical) || filepath.IsAbs(normalized) || hasWindowsDrivePrefix(normalized) {
		return "", fmt.Errorf("%w: expected a workspace-relative path", ErrUnsafePath)
	}
	for _, character := range logical {
		if character == 0 || unicode.IsControl(character) {
			return "", fmt.Errorf("%w: control character in path", ErrUnsafePath)
		}
	}
	cleaned := filepath.Clean(filepath.FromSlash(normalized))
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", ErrUnsafePath
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	resolved := filepath.Join(root, cleaned)
	parent := filepath.Dir(resolved)
	if output {
		info, statErr := os.Stat(parent)
		if statErr != nil || !info.IsDir() {
			return "", fmt.Errorf("%w: output parent directory must already exist", ErrUnsafePath)
		}
	}
	realParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, realParent)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", ErrUnsafePath
	}
	return filepath.Join(realParent, filepath.Base(resolved)), nil
}

package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestViewBoundaries(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "nested", "file.md"), "hello")

	t.Run("builds directory chain", func(t *testing.T) {
		view, err := New(root, 1024, false)
		if err != nil {
			t.Fatal(err)
		}
		dirs, err := view.DirectoriesToTarget("nested/file.go")
		if err != nil || !reflect.DeepEqual(dirs, []string{".", "nested"}) {
			t.Fatalf("dirs = %#v, err = %v", dirs, err)
		}
	})
	t.Run("rejects lexical escape", func(t *testing.T) {
		view, _ := New(root, 1024, false)
		_, err := view.Read("../outside.md")
		if !errors.Is(err, ErrOutsideWorkspace) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("rejects absolute path", func(t *testing.T) {
		view, _ := New(root, 1024, false)
		_, err := view.Read(filepath.Join(root, "nested", "file.md"))
		if !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("enforces size limit", func(t *testing.T) {
		view, _ := New(root, 2, false)
		_, err := view.Read("nested/file.md")
		if !errors.Is(err, ErrTooLarge) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("rejects symlink by default", func(t *testing.T) {
		if err := os.Symlink(filepath.Join(root, "nested", "file.md"), filepath.Join(root, "linked.md")); err != nil {
			t.Fatal(err)
		}
		view, _ := New(root, 1024, false)
		_, err := view.Read("linked.md")
		if !errors.Is(err, ErrSymlink) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("follows an opted-in internal symlink", func(t *testing.T) {
		view, _ := New(root, 1024, true)
		file, err := view.Read("linked.md")
		if err != nil || string(file.Bytes) != "hello" {
			t.Fatalf("file = %#v, err = %v", file, err)
		}
	})
	t.Run("rejects opted-in symlink escape", func(t *testing.T) {
		outside := filepath.Join(t.TempDir(), "outside.md")
		mustWrite(t, outside, "secret")
		if err := os.Symlink(outside, filepath.Join(root, "escape.md")); err != nil {
			t.Fatal(err)
		}
		view, _ := New(root, 1024, true)
		_, err := view.Read("escape.md")
		if !errors.Is(err, ErrOutsideWorkspace) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("maps only in-workspace absolute paths", func(t *testing.T) {
		root := t.TempDir()
		view, err := New(root, 1024, false)
		if err != nil {
			t.Fatal(err)
		}
		logical, err := view.LogicalFromAbsolute(filepath.Join(root, "docs", "context.md"))
		if err != nil || logical != "docs/context.md" {
			t.Fatalf("logical = %q, err = %v", logical, err)
		}
		if _, err := view.LogicalFromAbsolute(filepath.Join(t.TempDir(), "private.md")); !errors.Is(err, ErrOutsideWorkspace) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("lists immediate child directories with safe symlink state", func(t *testing.T) {
		mustWrite(t, filepath.Join(root, "skills", "one", "SKILL.md"), "one")
		if err := os.Symlink("one", filepath.Join(root, "skills", "linked")); err != nil {
			t.Fatal(err)
		}
		view, _ := New(root, 1024, false)
		directories, err := view.ChildDirectories("skills")
		if err != nil || len(directories) != 2 {
			t.Fatalf("directories = %#v, err = %v", directories, err)
		}
		if directories[0].Logical != "skills/linked" || !directories[0].Symlink || directories[0].Accessible {
			t.Fatalf("linked directory = %#v", directories[0])
		}
		if directories[1].Logical != "skills/one" || directories[1].Symlink || !directories[1].Accessible {
			t.Fatalf("normal directory = %#v", directories[1])
		}
		followed, _ := New(root, 1024, true)
		directories, err = followed.ChildDirectories("skills")
		if err != nil || !directories[0].Accessible {
			t.Fatalf("followed directories = %#v, err = %v", directories, err)
		}
	})
}

func TestNormalizeLabel(t *testing.T) {
	label, err := NormalizeLabel("  adaptive-ai-orchestrator  ")
	if err != nil || label != "adaptive-ai-orchestrator" {
		t.Fatalf("label = %q, err = %v", label, err)
	}
	for _, invalid := range []string{"private/project", `private\project`, "line\nbreak", "bidi\u202ename", strings.Repeat("a", 81)} {
		if _, err := NormalizeLabel(invalid); !errors.Is(err, ErrInvalidLabel) {
			t.Fatalf("label %q err = %v", invalid, err)
		}
	}
}

func mustWrite(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

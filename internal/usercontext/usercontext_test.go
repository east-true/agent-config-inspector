package usercontext

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserContextBoundaries(t *testing.T) {
	t.Run("Gemini configured filenames remain opaque", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		root := filepath.Join(home, ".gemini")
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(`{"context":{"fileName":"PRIVATE.md"}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "PRIVATE.md"), []byte("private"), 0o600); err != nil {
			t.Fatal(err)
		}
		sources, err := Load("google-gemini/cli", 1024)
		if err != nil || len(sources) != 1 || sources[0].Label != "<user-instruction-1>" || sources[0].Kind != "gemini-user-context" {
			t.Fatalf("sources = %#v, err = %v", sources, err)
		}
	})

	t.Run("provider directory symlink is rejected", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		outside := t.TempDir()
		if err := os.WriteFile(filepath.Join(outside, "GEMINI.md"), []byte("private"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(home, ".gemini")); err != nil {
			t.Fatal(err)
		}
		_, err := Load("google-gemini/cli", 1024)
		var safety *SafetyError
		if !errors.As(err, &safety) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestSafeDirectFilename(t *testing.T) {
	t.Parallel()
	for _, unsafe := range []string{"", ".", "..", "../private.md", `nested\\private.md`, "/private.md", "C:private.md", "bad\nname.md"} {
		if normalized, ok := safeDirectFilename(unsafe); ok {
			t.Errorf("safeDirectFilename(%q) = %q, true", unsafe, normalized)
		}
	}
	for _, safe := range []string{"GEMINI.md", "TEAM_CONTEXT.md", "지침.md"} {
		if normalized, ok := safeDirectFilename(safe); !ok || normalized != strings.TrimSpace(safe) {
			t.Errorf("safeDirectFilename(%q) = %q, %v", safe, normalized, ok)
		}
	}
}

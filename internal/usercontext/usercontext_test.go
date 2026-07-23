package usercontext

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserContextBoundaries(t *testing.T) {
	t.Run("Copilot custom home instructions remain opaque and ordered", func(t *testing.T) {
		home := t.TempDir()
		copilotRoot := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("COPILOT_HOME", copilotRoot)
		if err := os.WriteFile(filepath.Join(copilotRoot, "copilot-instructions.md"), []byte("private general"), 0o600); err != nil {
			t.Fatal(err)
		}
		instructionsRoot := filepath.Join(copilotRoot, "instructions", "nested")
		if err := os.MkdirAll(instructionsRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(instructionsRoot, "go.instructions.md"), []byte("---\napplyTo: \"**/*.go\"\n---\nprivate Go"), 0o600); err != nil {
			t.Fatal(err)
		}
		sources, err := Load("github-copilot/cli", 1024)
		if err != nil || len(sources) != 2 {
			t.Fatalf("sources = %#v, err = %v", sources, err)
		}
		if sources[0].Label != "<user-instruction-1>" || sources[0].Kind != "copilot-user-instruction" || sources[1].Label != "<user-rule-2>" || sources[1].Kind != "copilot-user-modular-instruction" {
			t.Fatalf("sources = %#v", sources)
		}
	})

	t.Run("Copilot modular instruction directory symlink is rejected", func(t *testing.T) {
		home := t.TempDir()
		copilotRoot := filepath.Join(home, ".copilot")
		outside := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("COPILOT_HOME", "")
		if err := os.MkdirAll(copilotRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(copilotRoot, "instructions")); err != nil {
			t.Fatal(err)
		}
		_, err := Load("github-copilot/cli", 1024)
		var safety *SafetyError
		if !errors.As(err, &safety) {
			t.Fatalf("err = %v", err)
		}
	})

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

	t.Run("Kimi brand and generic instructions remain opaque and ordered", func(t *testing.T) {
		home := t.TempDir()
		brandRoot := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("KIMI_CODE_HOME", brandRoot)
		if err := os.WriteFile(filepath.Join(brandRoot, "AGENTS.md"), []byte("brand private"), 0o600); err != nil {
			t.Fatal(err)
		}
		genericRoot := filepath.Join(home, ".agents")
		if err := os.MkdirAll(genericRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(genericRoot, "AGENTS.md"), []byte("generic private"), 0o600); err != nil {
			t.Fatal(err)
		}
		sources, err := Load("moonshotai-kimi-code/cli", 1024)
		if err != nil || len(sources) != 2 {
			t.Fatalf("sources = %#v, err = %v", sources, err)
		}
		if sources[0].Label != "<user-instruction-1>" || sources[0].Kind != "kimi-user-brand-instruction" || sources[1].Label != "<user-instruction-2>" || sources[1].Kind != "kimi-user-generic-instruction" {
			t.Fatalf("sources = %#v", sources)
		}
	})

	t.Run("Kimi empty uppercase generic instruction falls back to lowercase", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("KIMI_CODE_HOME", filepath.Join(home, ".custom-kimi"))
		genericRoot := filepath.Join(home, ".agents")
		if err := os.MkdirAll(genericRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(genericRoot, "AGENTS.md"), []byte(" \n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(genericRoot, "agents.md"), []byte("lower private"), 0o600); err != nil {
			t.Fatal(err)
		}
		sources, err := Load("moonshotai-kimi-code/cli", 1024)
		if err != nil || len(sources) != 1 || string(sources[0].Content) != "lower private" {
			t.Fatalf("sources = %#v, err = %v", sources, err)
		}
	})

	t.Run("Kimi custom home symlink is rejected", func(t *testing.T) {
		home := t.TempDir()
		outside := t.TempDir()
		brandRoot := filepath.Join(home, ".custom-kimi")
		t.Setenv("HOME", home)
		t.Setenv("KIMI_CODE_HOME", brandRoot)
		if err := os.WriteFile(filepath.Join(outside, "AGENTS.md"), []byte("private"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, brandRoot); err != nil {
			t.Fatal(err)
		}
		_, err := Load("moonshotai-kimi-code/cli", 1024)
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

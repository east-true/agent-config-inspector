package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	t.Run("version", func(t *testing.T) {
		code, stdout, _ := invoke(t, []string{"version"})
		if code != exitOK || !strings.Contains(stdout, "0.1.0-dev") {
			t.Fatalf("code = %d, stdout = %q", code, stdout)
		}
	})
	t.Run("scan empty workspace", func(t *testing.T) {
		code, stdout, stderr := invoke(t, []string{"scan", t.TempDir(), "--fail-on", "never"})
		if code != exitOK || stderr != "" || !strings.Contains(stdout, "predicted-empty") {
			t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
	})
	t.Run("warning threshold", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Codex only"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, _, _ := invoke(t, []string{"scan", root, "--fail-on", "warning"})
		if code != exitFinding {
			t.Fatalf("code = %d", code)
		}
	})
	t.Run("unsupported provider", func(t *testing.T) {
		code, _, stderr := invoke(t, []string{"scan", t.TempDir(), "--provider", "gemini"})
		if code != exitUnsupported || !strings.Contains(stderr, "unsupported provider") {
			t.Fatalf("code = %d, stderr = %q", code, stderr)
		}
	})
	t.Run("diff requires two providers", func(t *testing.T) {
		code, _, _ := invoke(t, []string{"diff", t.TempDir(), "--provider", "claude"})
		if code != exitUsage {
			t.Fatalf("code = %d", code)
		}
	})
	t.Run("analysis help succeeds", func(t *testing.T) {
		code, _, _ := invoke(t, []string{"scan", "--help"})
		if code != exitOK {
			t.Fatalf("code = %d", code)
		}
	})
	t.Run("workspace escape is refused", func(t *testing.T) {
		code, _, _ := invoke(t, []string{"scan", t.TempDir(), "--target", "../outside"})
		if code != exitSafety {
			t.Fatalf("code = %d", code)
		}
	})
	t.Run("json output", func(t *testing.T) {
		code, stdout, _ := invoke(t, []string{"explain", t.TempDir(), "--provider", "codex", "--format", "json"})
		if code != exitOK || !strings.Contains(stdout, `"schema_version": 1`) {
			t.Fatalf("code = %d, stdout = %q", code, stdout)
		}
	})
}

func invoke(t *testing.T, args []string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

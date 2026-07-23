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
		if code != exitOK || !strings.Contains(stdout, "0.3.0-dev") {
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
		code, _, stderr := invoke(t, []string{"scan", t.TempDir(), "--provider", "kimi"})
		if code != exitUnsupported || !strings.Contains(stderr, "unsupported provider") {
			t.Fatalf("code = %d, stderr = %q", code, stderr)
		}
	})
	t.Run("Gemini provider", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "GEMINI.md"), []byte("Run tests"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := invoke(t, []string{"explain", root, "--provider", "gemini"})
		if code != exitOK || stderr != "" || !strings.Contains(stdout, "google-gemini/cli") {
			t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
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
	t.Run("sarif output", func(t *testing.T) {
		code, stdout, stderr := invoke(t, []string{"scan", t.TempDir(), "--format", "sarif", "--fail-on", "never"})
		if code != exitOK || stderr != "" || !strings.Contains(stdout, `"version": "2.1.0"`) {
			t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
	})
	t.Run("unsupported output format", func(t *testing.T) {
		code, _, stderr := invoke(t, []string{"scan", t.TempDir(), "--format", "xml"})
		if code != exitUsage || !strings.Contains(stderr, "text, json, or sarif") {
			t.Fatalf("code = %d, stderr = %q", code, stderr)
		}
	})
	t.Run("pin and verify round trip", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Run tests"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := invoke(t, []string{"pin", root})
		if code != exitOK || stderr != "" {
			t.Fatalf("pin code = %d, stderr = %q", code, stderr)
		}
		lockBytes, err := os.ReadFile(filepath.Join(root, "agent-config-inspector.lock.json"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(lockBytes), "/home/") || strings.Contains(string(lockBytes), "user-instruction") {
			t.Fatalf("unsafe lockfile = %s", lockBytes)
		}
		code, stdout, stderr := invoke(t, []string{"verify", root})
		if code != exitOK || stderr != "" || !strings.Contains(stdout, "Snapshot: verified") {
			t.Fatalf("verify code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
	})
	t.Run("verify detects drift", func(t *testing.T) {
		root := t.TempDir()
		instruction := filepath.Join(root, "AGENTS.md")
		if err := os.WriteFile(instruction, []byte("Run tests"), 0o600); err != nil {
			t.Fatal(err)
		}
		if code, _, stderr := invoke(t, []string{"pin", root}); code != exitOK {
			t.Fatalf("pin code = %d, stderr = %q", code, stderr)
		}
		if err := os.WriteFile(instruction, []byte("Run all tests"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, stdout, _ := invoke(t, []string{"verify", root})
		if code != exitFinding || !strings.Contains(stdout, "ACI063") {
			t.Fatalf("code = %d, stdout = %q", code, stdout)
		}
	})
	t.Run("pin refuses user context", func(t *testing.T) {
		code, _, stderr := invoke(t, []string{"pin", t.TempDir(), "--include-user-context"})
		if code != exitSafety || !strings.Contains(stderr, "must not encode user context") {
			t.Fatalf("code = %d, stderr = %q", code, stderr)
		}
	})
	t.Run("pin refuses output escape", func(t *testing.T) {
		code, _, _ := invoke(t, []string{"pin", t.TempDir(), "--output", "../outside.lock.json"})
		if code != exitSafety {
			t.Fatalf("code = %d", code)
		}
	})
}

func invoke(t *testing.T, args []string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

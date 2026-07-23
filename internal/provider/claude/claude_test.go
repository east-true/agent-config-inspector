package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/east-true/agent-config-inspector/internal/provider"
	"github.com/east-true/agent-config-inspector/internal/workspace"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

func TestClaudeResolution(t *testing.T) {
	t.Run("root memory", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{"CLAUDE.md": "Root"}, "src/main.go")
		assertPaths(t, includedPaths(result), []string{"CLAUDE.md"})
	})
	t.Run("alternative project memory", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{".claude/CLAUDE.md": "Project"}, ".")
		assertPaths(t, includedPaths(result), []string{".claude/CLAUDE.md"})
	})
	t.Run("nested and local ordering", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			"CLAUDE.md": "Root", "CLAUDE.local.md": "Root local", "src/CLAUDE.md": "Nested", "src/CLAUDE.local.md": "Nested local",
		}, "src/main.go")
		assertPaths(t, includedPaths(result), []string{"CLAUDE.md", "CLAUDE.local.md", "src/CLAUDE.md", "src/CLAUDE.local.md"})
	})
	t.Run("relative import", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{"CLAUDE.md": "@docs/shared.md", "docs/shared.md": "Shared"}, ".")
		assertPaths(t, includedPaths(result), []string{"CLAUDE.md", "docs/shared.md"})
	})
	t.Run("import cycle is bounded", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{"CLAUDE.md": "@a.md", "a.md": "@CLAUDE.md"}, ".")
		if !findingExists(result.Findings, "ACI025") {
			t.Fatalf("findings = %#v", result.Findings)
		}
	})
	t.Run("missing import is partial evidence", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{"CLAUDE.md": "@missing.md"}, ".")
		if !findingExists(result.Findings, "ACI026") || len(result.ExcludedSources) != 1 {
			t.Fatalf("result = %#v", result)
		}
	})
	t.Run("external import is redacted", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{"CLAUDE.md": "@/home/alice/private.md"}, ".")
		if !findingExists(result.Findings, "ACI005") || len(result.ExcludedSources) != 1 || result.ExcludedSources[0].DisplayPath != "<external-import>" {
			t.Fatalf("result = %#v", result)
		}
	})
	t.Run("matching project rule", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			".claude/rules/go.md": "---\npaths: [\"src/**/*.go\"]\n---\nGo rule",
		}, "src/api/main.go")
		assertPaths(t, includedPaths(result), []string{".claude/rules/go.md"})
	})
	t.Run("nonmatching project rule", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			".claude/rules/go.md": "---\npaths: [\"src/**/*.go\"]\n---\nGo rule",
		}, "docs/readme.md")
		if len(result.IncludedSources) != 0 || len(result.ExcludedSources) != 1 {
			t.Fatalf("result = %#v", result)
		}
	})
	t.Run("malformed project rule glob is explicit", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			".claude/rules/bad.md": "---\npaths: [\"src/{go\"]\n---\nRule",
		}, "src/main.go")
		if !findingExists(result.Findings, "ACI027") || len(result.ExcludedSources) != 1 {
			t.Fatalf("result = %#v", result)
		}
	})
	t.Run("unconditional recursive rule", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{".claude/rules/backend/security.md": "Rule"}, ".")
		assertPaths(t, includedPaths(result), []string{".claude/rules/backend/security.md"})
	})
	t.Run("comments do not enter effective content", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{"CLAUDE.md": "Keep <!-- private note --> this"}, ".")
		if strings.Contains(result.IncludedSources[0].Content, "private note") {
			t.Fatalf("content = %q", result.IncludedSources[0].Content)
		}
	})
	t.Run("project memory is not duplicated for target under dot claude", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{".claude/CLAUDE.md": "Project"}, ".claude/settings.json")
		assertPaths(t, includedPaths(result), []string{".claude/CLAUDE.md"})
	})
	t.Run("opted in external source stays opaque", func(t *testing.T) {
		root := t.TempDir()
		view, err := workspace.New(root, 1<<20, false)
		if err != nil {
			t.Fatal(err)
		}
		result, err := New().Resolve(context.Background(), view, ".", provider.Options{ExternalSources: []provider.ExternalSource{{
			Label: "<user-instruction-1>", Kind: "claude-user-memory", Content: []byte("private preference"),
		}}})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.IncludedSources) != 1 || result.IncludedSources[0].LogicalPath != "" || result.IncludedSources[0].RawDigest != nil || result.IncludedSources[0].NormalizedDigest != nil {
			t.Fatalf("source = %#v", result.IncludedSources)
		}
	})
}

func resolveFixture(t *testing.T, files map[string]string, target string) agentconfig.Resolution {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	view, err := workspace.New(root, 1<<20, false)
	if err != nil {
		t.Fatal(err)
	}
	result, err := New().Resolve(context.Background(), view, target, provider.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func includedPaths(result agentconfig.Resolution) []string {
	paths := make([]string, 0, len(result.IncludedSources))
	for _, source := range result.IncludedSources {
		paths = append(paths, source.LogicalPath)
	}
	return paths
}

func assertPaths(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("included = %#v, want %#v", got, want)
	}
}

func findingExists(findings []agentconfig.Finding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

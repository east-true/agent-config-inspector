package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/east-true/agent-config-inspector/internal/provider"
	"github.com/east-true/agent-config-inspector/internal/workspace"
)

func TestCodexResolution(t *testing.T) {
	t.Run("root instruction", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{"AGENTS.md": "Root"}, "src/main.go")
		assertIncluded(t, resultPaths(result), []string{"AGENTS.md"})
	})
	t.Run("nested instruction order", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{"AGENTS.md": "Root", "src/AGENTS.md": "Nested"}, "src/main.go")
		assertIncluded(t, resultPaths(result), []string{"AGENTS.md", "src/AGENTS.md"})
	})
	t.Run("override wins within directory", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{"src/AGENTS.override.md": "Override", "src/AGENTS.md": "Nested"}, "src/main.go")
		assertIncluded(t, resultPaths(result), []string{"src/AGENTS.override.md"})
		if len(result.ExcludedSources) != 1 || result.ExcludedSources[0].LogicalPath != "src/AGENTS.md" {
			t.Fatalf("excluded = %#v", result.ExcludedSources)
		}
	})
	t.Run("empty override is skipped", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{"AGENTS.override.md": "  \n", "AGENTS.md": "Root"}, ".")
		assertIncluded(t, resultPaths(result), []string{"AGENTS.md"})
	})
	t.Run("configured fallback", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			".codex/config.toml": `project_doc_fallback_filenames = ["TEAM.md"]`, "TEAM.md": "Team",
		}, ".")
		assertIncluded(t, resultPaths(result), []string{"TEAM.md"})
	})
	t.Run("closest config wins", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			".codex/config.toml":     `project_doc_fallback_filenames = ["ROOT.md"]`,
			"src/.codex/config.toml": `project_doc_fallback_filenames = ["NEAR.md"]`,
			"ROOT.md":                "Root", "src/NEAR.md": "Near",
		}, "src/main.go")
		assertIncluded(t, resultPaths(result), []string{"src/NEAR.md"})
	})
	t.Run("combined budget excludes source", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			".codex/config.toml": "project_doc_max_bytes = 4", "AGENTS.md": "12345",
		}, ".")
		if len(result.IncludedSources) != 0 || !hasFinding(result.Findings, "ACI045") {
			t.Fatalf("result = %#v", result)
		}
	})
}

func resolveFixture(t *testing.T, files map[string]string, target string) (result struct {
	IncludedSources []struct{ LogicalPath string }
	ExcludedSources []struct{ LogicalPath string }
	Findings        []struct{ Code string }
}) {
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
	resolved, err := New().Resolve(context.Background(), view, target, provider.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range resolved.IncludedSources {
		result.IncludedSources = append(result.IncludedSources, struct{ LogicalPath string }{source.LogicalPath})
	}
	for _, source := range resolved.ExcludedSources {
		result.ExcludedSources = append(result.ExcludedSources, struct{ LogicalPath string }{source.LogicalPath})
	}
	for _, finding := range resolved.Findings {
		result.Findings = append(result.Findings, struct{ Code string }{finding.Code})
	}
	return result
}

func resultPaths(result struct {
	IncludedSources []struct{ LogicalPath string }
	ExcludedSources []struct{ LogicalPath string }
	Findings        []struct{ Code string }
}) []string {
	var paths []string
	for _, source := range result.IncludedSources {
		paths = append(paths, source.LogicalPath)
	}
	return paths
}

func assertIncluded(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("included = %#v, want %#v", got, want)
	}
}

func hasFinding(findings []struct{ Code string }, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

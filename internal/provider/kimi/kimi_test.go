package kimi

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

func TestKimiResolution(t *testing.T) {
	t.Run("loads branded and generic hierarchy root to target", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			".git": "marker", ".kimi-code/AGENTS.md": "Root brand", "AGENTS.md": "Root generic",
			"packages/.kimi-code/AGENTS.md": "Package brand", "packages/agents.md": "Package generic",
			"packages/app/AGENTS.md": "App generic", "other/AGENTS.md": "Sibling",
		}, "packages/app/main.ts", provider.Options{})
		assertPaths(t, includedPaths(result), []string{
			".kimi-code/AGENTS.md", "AGENTS.md", "packages/.kimi-code/AGENTS.md", "packages/agents.md", "packages/app/AGENTS.md",
		})
	})

	t.Run("nearest git marker is the project root", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			".git": "outer", "AGENTS.md": "Outer", "apps/site/.git": "inner", "apps/site/AGENTS.md": "Site",
		}, "apps/site/src/main.ts", provider.Options{})
		assertPaths(t, includedPaths(result), []string{"apps/site/AGENTS.md"})
	})

	t.Run("without git only the target directory is searched", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			"AGENTS.md": "Workspace", "packages/AGENTS.md": "Parent", "packages/app/AGENTS.md": "Target",
		}, "packages/app/main.ts", provider.Options{})
		assertPaths(t, includedPaths(result), []string{"packages/app/AGENTS.md"})
	})

	t.Run("uppercase generic filename wins", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{".git": "marker", "AGENTS.md": "Upper", "agents.md": "Lower"}, ".", provider.Options{})
		assertPaths(t, includedPaths(result), []string{"AGENTS.md"})
	})

	t.Run("empty uppercase filename falls back to lowercase", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{".git": "marker", "AGENTS.md": " \n", "agents.md": "Lower"}, ".", provider.Options{})
		assertPaths(t, includedPaths(result), []string{"agents.md"})
		if len(result.ExcludedSources) != 1 || !hasFinding(result.Findings, "ACI003") {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("frontmatter remains instruction content", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			".git": "marker", "AGENTS.md": "---\npaths:\n  - src/**\n---\nInstruction",
		}, ".", provider.Options{})
		if len(result.IncludedSources) != 1 || !strings.HasPrefix(result.IncludedSources[0].Content, "---\npaths:") || len(result.IncludedSources[0].Scope.Patterns) != 0 {
			t.Fatalf("source = %#v", result.IncludedSources)
		}
	})

	t.Run("at syntax remains plain instruction text", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{".git": "marker", "AGENTS.md": "@missing.md"}, ".", provider.Options{})
		if len(result.IncludedSources) != 1 || len(result.ExcludedSources) != 0 || hasFinding(result.Findings, "ACI026") {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("recommended budget warns without truncating", func(t *testing.T) {
		large := strings.Repeat("x", recommendedMaxBytes+1)
		result := resolveFixture(t, map[string]string{".git": "marker", "AGENTS.md": large}, ".", provider.Options{})
		if len(result.IncludedSources) != 1 || !hasFinding(result.Findings, "ACI045") || len(result.IncludedSources[0].Content) != len(large) {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("opted in user sources precede project sources and stay opaque", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{".git": "marker", "AGENTS.md": "Project"}, ".", provider.Options{
			ExternalSources: []provider.ExternalSource{
				{Label: "<user-instruction-1>", Kind: "kimi-user-brand-instruction", Content: []byte("Brand")},
				{Label: "<user-instruction-2>", Kind: "kimi-user-generic-instruction", Content: []byte("Generic")},
			},
		})
		assertPaths(t, includedPaths(result), []string{"", "", "AGENTS.md"})
		for _, source := range result.IncludedSources[:2] {
			if source.Origin != "user" || source.RawDigest != nil || source.LogicalPath != "" {
				t.Fatalf("source = %#v", source)
			}
		}
	})

	t.Run("empty opted in user sources are ignored", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{".git": "marker", "AGENTS.md": "Project"}, ".", provider.Options{
			ExternalSources: []provider.ExternalSource{
				{Label: "<user-instruction-1>", Kind: "kimi-user-brand-instruction", Content: []byte(" \n")},
			},
		})
		assertPaths(t, includedPaths(result), []string{"AGENTS.md"})
	})

	t.Run("symlinked project instruction is rejected by default", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, ".git", "marker")
		writeFixtureFile(t, root, "real.md", "Instruction")
		if err := os.Symlink(filepath.Join(root, "real.md"), filepath.Join(root, "AGENTS.md")); err != nil {
			t.Fatal(err)
		}
		result := resolveRoot(t, root, ".", provider.Options{})
		if !hasFinding(result.Findings, "ACI006") {
			t.Fatalf("findings = %#v", result.Findings)
		}
	})
}

func resolveFixture(t *testing.T, files map[string]string, target string, options provider.Options) agentconfig.Resolution {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		writeFixtureFile(t, root, name, content)
	}
	return resolveRoot(t, root, target, options)
}

func writeFixtureFile(t *testing.T, root, name, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func resolveRoot(t *testing.T, root, target string, options provider.Options) agentconfig.Resolution {
	t.Helper()
	view, err := workspace.New(root, 1<<20, false)
	if err != nil {
		t.Fatal(err)
	}
	result, err := New().Resolve(context.Background(), view, target, options)
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

func hasFinding(findings []agentconfig.Finding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

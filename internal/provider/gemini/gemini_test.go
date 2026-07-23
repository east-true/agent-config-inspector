package gemini

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/east-true/agent-config-inspector/internal/provider"
	"github.com/east-true/agent-config-inspector/internal/workspace"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

func TestGeminiResolution(t *testing.T) {
	t.Run("loads context from root to target", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			"GEMINI.md": "Root", "src/GEMINI.md": "Nested", "other/GEMINI.md": "Sibling",
		}, "src/main.go", provider.Options{})
		assertPaths(t, includedPaths(result), []string{"GEMINI.md", "src/GEMINI.md"})
	})

	t.Run("nearest git marker is the memory boundary", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			"GEMINI.md": "Outer", "packages/app/.git": "gitdir marker", "packages/app/GEMINI.md": "App",
		}, "packages/app/src/main.go", provider.Options{})
		assertPaths(t, includedPaths(result), []string{"packages/app/GEMINI.md"})
	})

	t.Run("configured filenames precede the retained default", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			".gemini/settings.json": `{"context":{"fileName":["CONTEXT.md","TEAM.md"]}}`,
			"CONTEXT.md":            "Context", "TEAM.md": "Team", "GEMINI.md": "Default",
		}, ".", provider.Options{})
		assertPaths(t, includedPaths(result), []string{"CONTEXT.md", "TEAM.md", "GEMINI.md"})
	})

	t.Run("custom boundary marker changes hierarchy ceiling", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			".gemini/settings.json": `{"context":{"memoryBoundaryMarkers":[".root"]}}`,
			"GEMINI.md":             "Outer", "packages/.root": "marker", "packages/GEMINI.md": "Package",
		}, "packages/src/main.go", provider.Options{})
		assertPaths(t, includedPaths(result), []string{"packages/GEMINI.md"})
	})

	t.Run("unsafe boundary marker is rejected", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			"app/.gemini/settings.json": `{"context":{"memoryBoundaryMarkers":["nested\\\\.root"]}}`,
			"GEMINI.md":                 "Outer", "app/.git": "marker", "app/GEMINI.md": "App",
		}, "app/src/main.go", provider.Options{})
		assertPaths(t, includedPaths(result), []string{"app/GEMINI.md"})
		if !hasFinding(result.Findings, "ACI023") {
			t.Fatalf("findings = %#v", result.Findings)
		}
	})

	t.Run("unsafe configured filename falls back safely", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			".gemini/settings.json": `{"context":{"fileName":"../private.md"}}`, "GEMINI.md": "Default",
		}, ".", provider.Options{})
		assertPaths(t, includedPaths(result), []string{"GEMINI.md"})
		if !hasFinding(result.Findings, "ACI023") {
			t.Fatalf("findings = %#v", result.Findings)
		}
	})

	t.Run("expands nested relative imports", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			"GEMINI.md": "@docs/context.md", "docs/context.md": "@shared.md", "docs/shared.md": "Shared",
		}, ".", provider.Options{})
		assertPaths(t, includedPaths(result), []string{"GEMINI.md", "docs/context.md", "docs/shared.md"})
	})

	t.Run("maps absolute imports only when they remain inside the workspace", func(t *testing.T) {
		root := t.TempDir()
		absolute := filepath.Join(root, "docs", "shared.md")
		writeFixtureFile(t, root, "GEMINI.md", "@"+absolute)
		writeFixtureFile(t, root, "docs/shared.md", "Shared")

		result := resolveRoot(t, root, ".", provider.Options{})
		assertPaths(t, includedPaths(result), []string{"GEMINI.md", "docs/shared.md"})
	})

	t.Run("ignores import-like text in fenced code", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			"GEMINI.md": "```text\n@private.md\n```\n@real.md", "private.md": "Private", "real.md": "Real",
		}, ".", provider.Options{})
		assertPaths(t, includedPaths(result), []string{"GEMINI.md", "real.md"})
	})

	t.Run("imports an extensionless path", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{"GEMINI.md": "@LICENSE", "LICENSE": "Terms"}, ".", provider.Options{})
		assertPaths(t, includedPaths(result), []string{"GEMINI.md", "LICENSE"})
	})

	t.Run("rejects URL imports without disclosing the URL", func(t *testing.T) {
		secretURL := "https://example.invalid/private-context.md"
		result := resolveFixture(t, map[string]string{"GEMINI.md": "@" + secretURL}, ".", provider.Options{})
		if !hasFinding(result.Findings, "ACI005") || len(result.ExcludedSources) != 1 {
			t.Fatalf("result = %#v", result)
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), secretURL) {
			t.Fatalf("external URL leaked in result: %#v", result)
		}
	})

	t.Run("import cycle is bounded", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{"GEMINI.md": "@a.md", "a.md": "@GEMINI.md"}, ".", provider.Options{})
		if !hasFinding(result.Findings, "ACI025") {
			t.Fatalf("findings = %#v", result.Findings)
		}
	})

	t.Run("missing import is explicit", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{"GEMINI.md": "@missing.md"}, ".", provider.Options{})
		if !hasFinding(result.Findings, "ACI026") || len(result.ExcludedSources) != 1 {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("project-boundary escape is redacted", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			"shared.md": "Private to outer workspace", "app/.git": "marker", "app/GEMINI.md": "@../shared.md",
		}, "app/src/main.go", provider.Options{})
		if !hasFinding(result.Findings, "ACI005") || len(result.ExcludedSources) != 1 || result.ExcludedSources[0].DisplayPath != "<external-import>" {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("provider import cap is enforced", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			"GEMINI.md": "@a.md", "a.md": "@b.md", "b.md": "@c.md", "c.md": "Deep",
		}, ".", provider.Options{MaxImportDepth: 2})
		assertPaths(t, includedPaths(result), []string{"GEMINI.md", "a.md", "b.md"})
		if !hasFinding(result.Findings, "ACI024") {
			t.Fatalf("findings = %#v", result.Findings)
		}
	})

	t.Run("empty context is excluded", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{"GEMINI.md": "  \n"}, ".", provider.Options{})
		if len(result.IncludedSources) != 0 || len(result.ExcludedSources) != 1 || !hasFinding(result.Findings, "ACI003") {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("opted in global context stays opaque", func(t *testing.T) {
		result := resolveFixture(t, nil, ".", provider.Options{ExternalSources: []provider.ExternalSource{{
			Label: "<user-instruction-1>", Kind: "gemini-user-context", Content: []byte("private preference"),
		}}})
		if len(result.IncludedSources) != 1 || result.IncludedSources[0].LogicalPath != "" || result.IncludedSources[0].RawDigest != nil {
			t.Fatalf("source = %#v", result.IncludedSources)
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

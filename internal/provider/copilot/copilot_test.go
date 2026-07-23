package copilot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/east-true/agent-config-inspector/internal/provider"
	"github.com/east-true/agent-config-inspector/internal/workspace"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

func TestCopilotResolution(t *testing.T) {
	t.Run("discovers standard and target-nested modular locations", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			".git":                            "marker",
			".github/copilot-instructions.md": "Repository",
			"AGENTS.md":                       "Agent",
			"packages/CLAUDE.md":              "Intermediate",
			"packages/.github/instructions/package.instructions.md":    "---\napplyTo: \"**/*.go\"\n---\nPackage",
			"packages/app/src/GEMINI.md":                               "Target compatible",
			".github/instructions/root-go.instructions.md":             "---\napplyTo: \"**/*.go\"\n---\nRoot Go",
			"packages/app/src/.github/instructions/go.instructions.md": "---\napplyTo: \"*.go\"\n---\nTarget Go",
		}, "packages/app/src/main.go", provider.Options{})
		assertPaths(t, includedPaths(result), []string{
			".github/copilot-instructions.md", "AGENTS.md", "packages/CLAUDE.md", "packages/app/src/GEMINI.md",
			".github/instructions/root-go.instructions.md", "packages/.github/instructions/package.instructions.md", "packages/app/src/.github/instructions/go.instructions.md",
		})
	})

	t.Run("nearest git root bounds standard discovery", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			"AGENTS.md": "Outer", "apps/service/.git": "marker", "apps/service/AGENTS.md": "Service",
		}, "apps/service/src/main.go", provider.Options{})
		assertPaths(t, includedPaths(result), []string{"apps/service/AGENTS.md"})
	})

	t.Run("symlinked git marker is reported", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Symlink(t.TempDir(), filepath.Join(root, ".git")); err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, root, "AGENTS.md", "Agent")
		view, err := workspace.New(root, 1<<20, false)
		if err != nil {
			t.Fatal(err)
		}
		result, err := New().Resolve(context.Background(), view, ".", provider.Options{})
		if err != nil {
			t.Fatal(err)
		}
		if !hasFinding(result.Findings, "ACI006") {
			t.Fatalf("findings = %#v", result.Findings)
		}
	})

	t.Run("applyTo supports comma separated target patterns", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			".github/instructions/typescript.instructions.md": "---\napplyTo: \"**/*.ts,**/*.tsx\"\n---\nTypeScript",
			".github/instructions/python.instructions.md":     "---\napplyTo: \"**/*.py\"\n---\nPython",
		}, "src/App.tsx", provider.Options{})
		assertPaths(t, includedPaths(result), []string{".github/instructions/typescript.instructions.md"})
		if len(result.ExcludedSources) != 1 || result.ExcludedSources[0].LogicalPath != ".github/instructions/python.instructions.md" {
			t.Fatalf("excluded = %#v", result.ExcludedSources)
		}
	})

	t.Run("missing applyTo is explicit and partial-capable", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			".github/instructions/unknown.instructions.md": "No frontmatter",
		}, "src/main.go", provider.Options{})
		if len(result.IncludedSources) != 0 || len(result.ExcludedSources) != 1 || !hasFinding(result.Findings, "ACI027") {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("empty instruction is discovered but excluded", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{"AGENTS.md": " \n"}, ".", provider.Options{})
		if len(result.IncludedSources) != 0 || len(result.ExcludedSources) != 1 || !hasFinding(result.Findings, "ACI003") {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("expands recursive imports only for supported source families", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			".github/copilot-instructions.md":         "@docs/first.md",
			".github/docs/first.md":                   "@second.md",
			".github/docs/second.md":                  "Imported",
			"GEMINI.md":                               "@gemini-only.md",
			"gemini-only.md":                          "Must not expand",
			".github/instructions/go.instructions.md": "---\napplyTo: \"**/*.go\"\n---\n@modular-only.md",
			".github/instructions/modular-only.md":    "Must not expand",
		}, "src/main.go", provider.Options{})
		assertPaths(t, includedPaths(result), []string{
			".github/copilot-instructions.md", ".github/docs/first.md", ".github/docs/second.md", "GEMINI.md", ".github/instructions/go.instructions.md",
		})
	})

	t.Run("ignores inline and fenced at-path text", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			"AGENTS.md": "See @inline.md\n```text\n@fenced.md\n```\n@real.md",
			"inline.md": "Inline", "fenced.md": "Fenced", "real.md": "Real",
		}, ".", provider.Options{})
		assertPaths(t, includedPaths(result), []string{"AGENTS.md", "real.md"})
	})

	t.Run("external imports are rejected without disclosing their path", func(t *testing.T) {
		for _, secretPath := range []string{
			"~/private-copilot-context.md",
			"/private-copilot-context.md",
			"https://example.invalid/private-copilot-context.md",
			`C:\Users\private-copilot-context.md`,
		} {
			result := resolveFixture(t, map[string]string{"AGENTS.md": "@" + secretPath}, ".", provider.Options{})
			if !hasFinding(result.Findings, "ACI005") || len(result.ExcludedSources) != 1 || result.ExcludedSources[0].DisplayPath != "<external-import>" {
				t.Fatalf("result = %#v", result)
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), secretPath) || strings.Contains(string(encoded), strings.ReplaceAll(secretPath, `\`, `\\`)) {
				t.Fatalf("external path leaked: %s", encoded)
			}
		}
	})

	t.Run("nested repository escape is rejected", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			"private.md": "Outer", "app/.git": "marker", "app/AGENTS.md": "@../private.md",
		}, "app/src/main.go", provider.Options{})
		if !hasFinding(result.Findings, "ACI005") || len(result.ExcludedSources) != 1 || result.ExcludedSources[0].DisplayPath != "<external-import>" {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("import cycle and depth are bounded", func(t *testing.T) {
		cycle := resolveFixture(t, map[string]string{"AGENTS.md": "@a.md", "a.md": "@AGENTS.md"}, ".", provider.Options{})
		if !hasFinding(cycle.Findings, "ACI025") {
			t.Fatalf("findings = %#v", cycle.Findings)
		}
		depth := resolveFixture(t, map[string]string{
			"AGENTS.md": "@a.md", "a.md": "@b.md", "b.md": "@c.md", "c.md": "Deep",
		}, ".", provider.Options{MaxImportDepth: 2})
		assertPaths(t, includedPaths(depth), []string{"AGENTS.md", "a.md", "b.md"})
		if !hasFinding(depth.Findings, "ACI024") {
			t.Fatalf("findings = %#v", depth.Findings)
		}
	})

	t.Run("identical general instructions are deduplicated", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			".github/copilot-instructions.md": "Same", "AGENTS.md": "Same", "GEMINI.md": "Different",
		}, ".", provider.Options{})
		assertPaths(t, includedPaths(result), []string{".github/copilot-instructions.md", "GEMINI.md"})
		if len(result.ExcludedSources) != 1 || result.ExcludedSources[0].LogicalPath != "AGENTS.md" || !hasFinding(result.Findings, "ACI041") {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("modular instructions do not use general-source deduplication", func(t *testing.T) {
		result := resolveFixture(t, map[string]string{
			"AGENTS.md": "Same",
			".github/instructions/go.instructions.md": "---\napplyTo: \"**/*.go\"\n---\nSame",
		}, "src/main.go", provider.Options{})
		assertPaths(t, includedPaths(result), []string{"AGENTS.md", ".github/instructions/go.instructions.md"})
	})

	t.Run("opted in user instructions remain opaque and path scoped", func(t *testing.T) {
		result := resolveFixture(t, nil, "src/main.go", provider.Options{ExternalSources: []provider.ExternalSource{
			{Label: "<user-instruction-1>", Kind: "copilot-user-instruction", Content: []byte("Private\n@nested.md")},
			{Label: "<user-rule-1>", Kind: "copilot-user-modular-instruction", Content: []byte("---\napplyTo: \"**/*.go\"\n---\nPrivate Go")},
			{Label: "<user-rule-2>", Kind: "copilot-user-modular-instruction", Content: []byte("---\napplyTo: \"**/*.py\"\n---\nPrivate Python")},
		}})
		if len(result.IncludedSources) != 2 || len(result.ExcludedSources) != 1 || !hasFinding(result.Findings, "ACI032") {
			t.Fatalf("result = %#v", result)
		}
		for _, source := range append(result.IncludedSources, result.ExcludedSources...) {
			if source.Origin == "user" && (source.LogicalPath != "" || source.RawDigest != nil || source.NormalizedDigest != nil) {
				t.Fatalf("user source metadata = %#v", source)
			}
		}
	})
}

func TestDocumentedCopilotGlobs(t *testing.T) {
	tests := []struct {
		pattern string
		target  string
		want    bool
	}{
		{"*", "main.py", true},
		{"*", "src/main.py", false},
		{"**", "src/nested/main.py", true},
		{"**/*", "main.py", true},
		{"*.py", "main.py", true},
		{"*.py", "src/main.py", false},
		{"**/*.py", "main.py", true},
		{"**/*.py", "src/main.py", true},
		{"src/*.py", "src/main.py", true},
		{"src/*.py", "src/nested/main.py", false},
		{"src/**/*.py", "src/main.py", true},
		{"src/**/*.py", "src/nested/main.py", true},
		{"**/subdir/**/*.py", "deep/parent/subdir/nested/main.py", true},
		{"**/subdir/**/*.py", "deep/parent/main.py", false},
	}
	for _, test := range tests {
		name := strings.ReplaceAll(test.pattern+"-"+test.target, "/", "_")
		t.Run(name, func(t *testing.T) {
			got, err := matchesAny([]string{test.pattern}, test.target, ".")
			if err != nil || got != test.want {
				t.Fatalf("matchesAny(%q, %q) = %v, %v", test.pattern, test.target, got, err)
			}
		})
	}
}

func TestCopilotBraceApplyTo(t *testing.T) {
	result := resolveFixture(t, map[string]string{
		".github/instructions/web.instructions.md": "---\napplyTo: \"src/**/*.{ts,tsx}\"\n---\nWeb",
	}, "src/components/App.tsx", provider.Options{})
	assertPaths(t, includedPaths(result), []string{".github/instructions/web.instructions.md"})
}

func resolveFixture(t *testing.T, files map[string]string, target string, options provider.Options) agentconfig.Resolution {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		writeFixtureFile(t, root, name, content)
	}
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

func includedPaths(result agentconfig.Resolution) []string {
	paths := make([]string, 0, len(result.IncludedSources))
	for _, source := range result.IncludedSources {
		paths = append(paths, source.LogicalPath)
	}
	return paths
}

func assertPaths(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
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

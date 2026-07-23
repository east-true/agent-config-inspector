package parser

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseMarkdown(t *testing.T) {
	t.Run("normalizes BOM and line endings", func(t *testing.T) {
		parsed := ParseMarkdown("s", []byte("\ufeff# Title\r\n\r\nText  \r\n"), false)
		if parsed.Normalized != "# Title\n\nText" {
			t.Fatalf("normalized = %q", parsed.Normalized)
		}
	})
	t.Run("parses inline paths frontmatter", func(t *testing.T) {
		parsed := ParseMarkdown("s", []byte("---\npaths: [\"src/**/*.go\", 'cmd/*']\n---\nRule"), false)
		want := []string{"cmd/*", "src/**/*.go"}
		if !reflect.DeepEqual(parsed.Paths, want) {
			t.Fatalf("paths = %#v", parsed.Paths)
		}
	})
	t.Run("parses list paths frontmatter", func(t *testing.T) {
		parsed := ParseMarkdown("s", []byte("---\npaths:\n  - src/**\n  - tests/**\n---\nRule"), false)
		if len(parsed.Paths) != 2 {
			t.Fatalf("paths = %#v", parsed.Paths)
		}
	})
	t.Run("finds imports in encounter order", func(t *testing.T) {
		parsed := ParseMarkdown("s", []byte("See @docs/b.md then @docs/a.md and @docs/b.md"), false)
		want := []string{"docs/b.md", "docs/a.md"}
		if !reflect.DeepEqual(parsed.Imports, want) {
			t.Fatalf("imports = %#v", parsed.Imports)
		}
	})
	t.Run("skips imports in code spans and fences", func(t *testing.T) {
		parsed := ParseMarkdown("s", []byte("`@private.md`\n```\n@fenced.md\n```\n@real.md"), false)
		if !reflect.DeepEqual(parsed.Imports, []string{"real.md"}) {
			t.Fatalf("imports = %#v", parsed.Imports)
		}
	})
	t.Run("strips comments outside code fences", func(t *testing.T) {
		parsed := ParseMarkdown("s", []byte("before <!-- hidden --> after\n```html\n<!-- kept -->\n```"), true)
		if strings.Contains(parsed.Content, "hidden") || !strings.Contains(parsed.Content, "<!-- kept -->") {
			t.Fatalf("content = %q", parsed.Content)
		}
	})
	t.Run("extracts command signal", func(t *testing.T) {
		parsed := ParseMarkdown("s", []byte("Run `go test ./...` before review."), false)
		if len(parsed.Units) != 1 || parsed.Units[0].Command != "go test ./..." {
			t.Fatalf("units = %#v", parsed.Units)
		}
	})
	t.Run("extracts multilingual prohibition signal", func(t *testing.T) {
		parsed := ParseMarkdown("s", []byte("Never publish credentials.\n민감정보를 커밋하지 마세요."), false)
		if len(parsed.Units) != 2 || !parsed.Units[0].Prohibition || !parsed.Units[1].Prohibition {
			t.Fatalf("units = %#v", parsed.Units)
		}
	})
}

func TestPlainMarkdownPreservesFrontmatter(t *testing.T) {
	raw := []byte("---\npaths:\n  - src/**\n---\nInstruction")
	parsed := ParsePlainMarkdown("source", raw, false)
	if len(parsed.Paths) != 0 || !strings.HasPrefix(parsed.Content, "---\npaths:") {
		t.Fatalf("parsed = %#v", parsed)
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		target  string
		want    bool
	}{
		{"recursive extension", "**/*.ts", "src/api/user.ts", true},
		{"recursive extension at root", "**/*.ts", "user.ts", true},
		{"directory tree", "src/**/*", "src/api/user.ts", true},
		{"directory tree mismatch", "src/**/*", "tests/user.ts", false},
		{"root only", "*.md", "README.md", true},
		{"root only nested mismatch", "*.md", "docs/README.md", false},
		{"single directory wildcard", "src/*.go", "src/main.go", true},
		{"single directory wildcard nested mismatch", "src/*.go", "src/api/main.go", false},
		{"brace expansion", "src/*.{ts,tsx}", "src/App.tsx", true},
		{"question mark", "cmd/?.go", "cmd/a.go", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := MatchGlob(test.pattern, test.target)
			if err != nil || got != test.want {
				t.Fatalf("MatchGlob(%q, %q) = %v, %v", test.pattern, test.target, got, err)
			}
		})
	}
}

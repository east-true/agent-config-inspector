package registry

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicProviderRegistry(t *testing.T) {
	registry := Builtin()
	identities := registry.Identities()
	want := []string{"anthropic-claude-code/cli", "github-copilot/cli", "google-gemini/cli", "moonshotai-kimi-code/cli", "openai-codex/cli"}
	if len(identities) != len(want) {
		t.Fatalf("identities = %#v", identities)
	}
	for index, identity := range identities {
		if identity.ID != want[index] {
			t.Fatalf("identity[%d] = %q, want %q", index, identity.ID, want[index])
		}
	}

	for _, unsupportedID := range []string{"grok", "github-copilot/vscode", "github-copilot/cloud-agent", "github-copilot/code-review"} {
		t.Run("unsupported-"+unsupportedID, func(t *testing.T) {
			_, err := registry.Get(unsupportedID)
			var unsupported *UnsupportedError
			if !errors.As(err, &unsupported) {
				t.Fatalf("err = %v", err)
			}
		})
	}

	for alias, wantID := range map[string]string{
		"copilot":   "github-copilot/cli",
		"kimi":      "moonshotai-kimi-code/cli",
		"kimi-code": "moonshotai-kimi-code/cli",
	} {
		t.Run("alias-"+alias, func(t *testing.T) {
			adapter, err := registry.Get(alias)
			if err != nil || adapter.Identity().ID != wantID {
				t.Fatalf("identity = %#v, err = %v", adapter, err)
			}
		})
	}

	repositoryRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	publicFiles := []string{
		"README.md",
		filepath.Join("docs", "support-matrix.md"),
		filepath.Join("schemas", "report.schema.json"),
		filepath.Join("schemas", "snapshot.schema.json"),
	}
	for _, publicFile := range publicFiles {
		t.Run("documented-"+strings.ReplaceAll(publicFile, string(filepath.Separator), "-"), func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(repositoryRoot, publicFile))
			if err != nil {
				t.Fatal(err)
			}
			for _, providerID := range want {
				if !strings.Contains(string(content), providerID) {
					t.Fatalf("%s does not contain %s", publicFile, providerID)
				}
			}
		})
	}
}

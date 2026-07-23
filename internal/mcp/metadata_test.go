package mcp

import (
	"strings"
	"testing"
)

func TestParseCodexSourceRejectsUnterminatedQuotedTableWithoutPanic(t *testing.T) {
	if _, err := parseCodexSource([]byte("[mcp_servers.']\ncommand = 'private'\n"), ".codex/config.toml", ".", 42); err == nil {
		t.Fatal("expected an invalid quoted table error")
	}
}

func TestParseClaudeSourceBoundsJSONNesting(t *testing.T) {
	raw := []byte(strings.Repeat(`{"nested":`, 130) + `null` + strings.Repeat(`}`, 130))
	if _, err := parseClaudeSource(raw, ".mcp.json", int64(len(raw))); err == nil {
		t.Fatal("expected the JSON nesting bound to reject the source")
	}
}

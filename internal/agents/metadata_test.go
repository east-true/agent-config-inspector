package agents

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseClaudeAgent(t *testing.T) {
	raw := []byte("---\nname: security-reviewer\ndescription: >\n  Reviews changes without\n  exposing private context.\ntools: Read, Grep\nmcpServers:\n  - docs\n---\nPRIVATE_CLAUDE_INSTRUCTIONS\n")
	parsed, err := parseClaudeAgent(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.name != "security-reviewer" || parsed.description != "Reviews changes without exposing private context." || parsed.instructions != "PRIVATE_CLAUDE_INSTRUCTIONS" {
		t.Fatalf("parsed = %#v", parsed)
	}
	if !reflect.DeepEqual(parsed.declaredCapabilities, []string{"mcpServers", "tools"}) {
		t.Fatalf("capabilities = %#v", parsed.declaredCapabilities)
	}
}

func TestParseClaudeAgentRejectsMalformedFrontmatter(t *testing.T) {
	for _, raw := range []string{
		"name: reviewer\n",
		"---\nname: reviewer\n",
		"---\nname: reviewer\nname: duplicate\n---\n",
		"---\ndescription: [not, scalar]\n---\n",
	} {
		if _, err := parseClaudeAgent([]byte(raw)); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}

func TestParseCodexAgent(t *testing.T) {
	raw := []byte("name = 'repository_reviewer'\ndescription = \"\"\"Reviews repository changes.\"\"\"\ndeveloper_instructions = '''\nPRIVATE_CODEX_INSTRUCTIONS\n'''\nmodel = \"private-model-value\"\n[mcp_servers.docs]\ncommand = \"private-command\"\n")
	parsed, err := parseCodexAgent(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.name != "repository_reviewer" || parsed.description != "Reviews repository changes." || !strings.Contains(parsed.instructions, "PRIVATE_CODEX_INSTRUCTIONS") {
		t.Fatalf("parsed = %#v", parsed)
	}
	if !reflect.DeepEqual(parsed.declaredCapabilities, []string{"mcp_servers", "model"}) {
		t.Fatalf("capabilities = %#v", parsed.declaredCapabilities)
	}
}

func TestParseCodexAgentRejectsNonStringAndUnterminatedValues(t *testing.T) {
	for _, raw := range []string{
		"name = 42\n",
		"name = \"\"\"unterminated\n",
		"name = 'one'\nname = 'two'\n",
	} {
		if _, err := parseCodexAgent([]byte(raw)); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}

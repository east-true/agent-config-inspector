package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/east-true/agent-config-inspector/internal/provider/registry"
	"github.com/east-true/agent-config-inspector/internal/workspace"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

func TestInventoryProviderSourcesAndRedaction(t *testing.T) {
	root := t.TempDir()
	writeMCP(t, root, ".mcp.json", `{
  "mcpServers": {
    "claude-local": {
      "type": "stdio",
      "command": "CLAUDE_PRIVATE_COMMAND",
      "args": ["CLAUDE_PRIVATE_ARG"],
      "env": {"CLAUDE_PRIVATE_ENV_NAME": "CLAUDE_PRIVATE_ENV_VALUE"}
    },
    "claude-remote": {
      "type": "http",
      "url": "https://PRIVATE_CLAUDE_URL.invalid",
      "headers": {"PRIVATE_HEADER_NAME": "PRIVATE_HEADER_VALUE"}
    }
  }
}`)
	writeMCP(t, root, ".codex/config.toml", `
[mcp_servers.codex_local]
command = "CODEX_PRIVATE_COMMAND"
args = [
  "CODEX_PRIVATE_ARG",
]
[mcp_servers.codex_local.env]
CODEX_PRIVATE_ENV_NAME = "CODEX_PRIVATE_ENV_VALUE"

[mcp_servers.codex_remote]
url = "https://PRIVATE_CODEX_URL.invalid"
[mcp_servers.codex_remote.http_headers]
PRIVATE_HEADER_NAME = "PRIVATE_HEADER_VALUE"
`)

	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.MCPInventoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || report.Privacy.UserContextScanned || report.Privacy.SensitiveOutput || len(report.Results) != 2 {
		t.Fatalf("report = %#v", report)
	}
	claude := resultFor(t, report, claudeID)
	codex := resultFor(t, report, codexID)
	if len(claude.AvailableServers) != 2 || claude.AvailableServers[0].Transport != "stdio" || !claude.AvailableServers[0].Executable || !claude.AvailableServers[0].CredentialFieldsPresent {
		t.Fatalf("Claude servers = %#v", claude.AvailableServers)
	}
	if len(codex.AvailableServers) != 2 || codex.AvailableServers[0].Transport != "stdio" || !codex.AvailableServers[1].CredentialFieldsPresent {
		t.Fatalf("Codex servers = %#v", codex.AvailableServers)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var textOutput bytes.Buffer
	if err := WriteText(&textOutput, report); err != nil {
		t.Fatal(err)
	}
	output := string(encoded) + textOutput.String()
	for _, secret := range []string{
		"CLAUDE_PRIVATE_COMMAND", "CLAUDE_PRIVATE_ARG", "CLAUDE_PRIVATE_ENV_NAME", "CLAUDE_PRIVATE_ENV_VALUE",
		"PRIVATE_CLAUDE_URL", "PRIVATE_HEADER_NAME", "PRIVATE_HEADER_VALUE", "CODEX_PRIVATE_COMMAND",
		"CODEX_PRIVATE_ARG", "CODEX_PRIVATE_ENV_NAME", "CODEX_PRIVATE_ENV_VALUE", "PRIVATE_CODEX_URL", root,
	} {
		if strings.Contains(output, secret) {
			t.Fatalf("inventory leaked %q: %s", secret, output)
		}
	}
}

func TestCodexInventoryDeepMergesSelectedTargetLayers(t *testing.T) {
	root := t.TempDir()
	writeMCP(t, root, ".codex/config.toml", `
[mcp_servers.docs]
command = "PRIVATE_ROOT_COMMAND"
required = true
[mcp_servers.docs.env]
PRIVATE_ROOT_ENV = "PRIVATE_ROOT_VALUE"
`)
	writeMCP(t, root, "packages/api/.codex/config.toml", `
[mcp_servers.docs]
enabled = false
tool_timeout_sec = 20
`)

	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.MCPInventoryOptions{Providers: []string{"codex"}, Targets: []string{"packages/api/main.go"}})
	if err != nil {
		t.Fatal(err)
	}
	result := resultFor(t, report, codexID)
	if len(result.AvailableServers) != 0 || len(result.ExcludedServers) != 1 {
		t.Fatalf("result = %#v", result)
	}
	server := result.ExcludedServers[0]
	if server.Status != "disabled" || server.Transport != "stdio" || server.Enabled || !server.Required || len(server.ContributingSources) != 2 || server.DisplayPath != "packages/api/.codex/config.toml" {
		t.Fatalf("server = %#v", server)
	}
	if !hasFinding(result.Findings, "ACI094", "info") || !hasFinding(result.Findings, "ACI021", "info") {
		t.Fatalf("findings = %#v", result.Findings)
	}
	encoded, _ := json.Marshal(report)
	for _, secret := range []string{"PRIVATE_ROOT_COMMAND", "PRIVATE_ROOT_ENV", "PRIVATE_ROOT_VALUE", root} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("inventory leaked %q: %s", secret, encoded)
		}
	}
}

func TestClaudeTransportValidationAndReservedName(t *testing.T) {
	root := t.TempDir()
	writeMCP(t, root, ".mcp.json", `{"mcpServers": {
  "without-type": {"url": "https://PRIVATE.invalid"},
  "empty-remote": {"type": "sse", "url": ""},
  "workspace": {"type": "stdio", "command": "PRIVATE_COMMAND"},
  "websocket": {"type": "ws", "url": "wss://PRIVATE.invalid"}
}}`)
	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.MCPInventoryOptions{Providers: []string{"claude"}})
	if err != nil {
		t.Fatal(err)
	}
	result := resultFor(t, report, claudeID)
	if len(result.AvailableServers) != 1 || result.AvailableServers[0].Name != "websocket" || result.AvailableServers[0].Transport != "ws" || len(result.ExcludedServers) != 3 {
		t.Fatalf("result = %#v", result)
	}
	if !hasFinding(result.Findings, "ACI090", "error") {
		t.Fatalf("findings = %#v", result.Findings)
	}
}

func TestClaudeDuplicateJSONFieldRejectsSource(t *testing.T) {
	root := t.TempDir()
	writeMCP(t, root, ".mcp.json", `{"mcpServers":{"one":{"command":"PRIVATE_ONE","command":"PRIVATE_TWO"}}}`)
	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.MCPInventoryOptions{Providers: []string{"claude"}})
	if err != nil {
		t.Fatal(err)
	}
	result := resultFor(t, report, claudeID)
	if report.Complete || result.State != "partial" || len(result.AvailableServers) != 0 || !hasFinding(result.Findings, "ACI090", "error") {
		t.Fatalf("report = %#v", report)
	}
}

func TestCodexQuotedNameAndTransportConflict(t *testing.T) {
	root := t.TempDir()
	writeMCP(t, root, ".codex/config.toml", `
[mcp_servers."docs.search"]
command = "PRIVATE_COMMAND"
`)
	writeMCP(t, root, "nested/.codex/config.toml", `
[mcp_servers."docs.search"]
url = "https://PRIVATE.invalid"
`)
	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.MCPInventoryOptions{Providers: []string{"codex"}, Targets: []string{"nested/file.go"}})
	if err != nil {
		t.Fatal(err)
	}
	result := resultFor(t, report, codexID)
	if len(result.AvailableServers) != 0 || len(result.ExcludedServers) != 1 || result.ExcludedServers[0].Name != "docs.search" || result.ExcludedServers[0].Status != "invalid" || !hasFinding(result.Findings, "ACI090", "error") {
		t.Fatalf("result = %#v", result)
	}
}

func TestInventoryMarksOversizedSourceIncomplete(t *testing.T) {
	root := t.TempDir()
	writeMCP(t, root, ".mcp.json", `{"mcpServers":{"one":{"command":"PRIVATE_COMMAND"}}}`)
	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.MCPInventoryOptions{Providers: []string{"claude"}, MaxSourceBytes: 16})
	if err != nil {
		t.Fatal(err)
	}
	result := resultFor(t, report, claudeID)
	if report.Complete || result.State != "partial" || !hasFinding(result.Findings, "ACI095", "error") {
		t.Fatalf("report = %#v", report)
	}
}

func TestCodexCloserLayerCanRepairInvalidBoolean(t *testing.T) {
	root := t.TempDir()
	writeMCP(t, root, ".codex/config.toml", "[mcp_servers.docs]\ncommand = 'PRIVATE_COMMAND'\nenabled = 'PRIVATE_INVALID_BOOL'\n")
	writeMCP(t, root, "nested/.codex/config.toml", "[mcp_servers.docs]\nenabled = true\n")
	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.MCPInventoryOptions{Providers: []string{"codex"}, Targets: []string{"nested/file.go"}})
	if err != nil {
		t.Fatal(err)
	}
	result := resultFor(t, report, codexID)
	if len(result.AvailableServers) != 1 || result.AvailableServers[0].Status != "configured" || result.AvailableServers[0].Transport != "stdio" {
		t.Fatalf("result = %#v", result)
	}
}

func TestMCPInventoryRejectsUnsafeNameWithoutEchoingIt(t *testing.T) {
	root := t.TempDir()
	writeMCP(t, root, ".mcp.json", `{"mcpServers":{"unsafe\u001bname":{"command":"PRIVATE_COMMAND"}}}`)
	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.MCPInventoryOptions{Providers: []string{"claude"}})
	if err != nil {
		t.Fatal(err)
	}
	result := resultFor(t, report, claudeID)
	if len(result.AvailableServers) != 0 || len(result.ExcludedServers) != 1 || result.ExcludedServers[0].Name != "<unsafe-server-name>" {
		t.Fatalf("result = %#v", result)
	}
	encoded, _ := json.Marshal(report)
	if bytes.Contains(encoded, []byte{0x1b}) || bytes.Contains(encoded, []byte(`unsafe\u001bname`)) {
		t.Fatalf("unsafe name leaked: %s", encoded)
	}
}

func TestMCPInventorySymlinkPolicy(t *testing.T) {
	root := t.TempDir()
	writeMCP(t, root, "shared/mcp.json", `{"mcpServers":{"one":{"command":"PRIVATE_COMMAND"}}}`)
	if err := os.Symlink(filepath.Join("shared", "mcp.json"), filepath.Join(root, ".mcp.json")); err != nil {
		t.Fatal(err)
	}
	service := New(registry.Builtin())
	_, err := service.Inventory(context.Background(), root, agentconfig.MCPInventoryOptions{Providers: []string{"claude"}})
	if !errors.Is(err, workspace.ErrSymlink) {
		t.Fatalf("safe err = %v", err)
	}
	report, err := service.Inventory(context.Background(), root, agentconfig.MCPInventoryOptions{Providers: []string{"claude"}, FollowSymlinks: true})
	if err != nil {
		t.Fatal(err)
	}
	if result := resultFor(t, report, claudeID); len(result.AvailableServers) != 1 {
		t.Fatalf("followed result = %#v", result)
	}

	outside := t.TempDir()
	writeMCP(t, outside, "mcp.json", `{"mcpServers":{}}`)
	if err := os.Remove(filepath.Join(root, ".mcp.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "mcp.json"), filepath.Join(root, ".mcp.json")); err != nil {
		t.Fatal(err)
	}
	_, err = service.Inventory(context.Background(), root, agentconfig.MCPInventoryOptions{Providers: []string{"claude"}, FollowSymlinks: true})
	if !errors.Is(err, workspace.ErrOutsideWorkspace) {
		t.Fatalf("outside err = %v", err)
	}
}

func resultFor(t *testing.T, report agentconfig.MCPInventoryReport, providerID string) agentconfig.MCPInventoryResolution {
	t.Helper()
	for _, result := range report.Results {
		if result.Provider.ID == providerID {
			return result
		}
	}
	t.Fatalf("provider %s not found in %#v", providerID, report.Results)
	return agentconfig.MCPInventoryResolution{}
}

func hasFinding(findings []agentconfig.Finding, code, severity string) bool {
	for _, finding := range findings {
		if finding.Code == code && finding.Severity == severity {
			return true
		}
	}
	return false
}

func writeMCP(t *testing.T, root, logical, content string) {
	t.Helper()
	name := filepath.Join(root, filepath.FromSlash(logical))
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

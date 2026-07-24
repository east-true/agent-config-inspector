package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	t.Run("version", func(t *testing.T) {
		code, stdout, _ := invoke(t, []string{"version"})
		if code != exitOK || !strings.Contains(stdout, "0.9.0-dev") {
			t.Fatalf("code = %d, stdout = %q", code, stdout)
		}
	})
	t.Run("scan empty workspace", func(t *testing.T) {
		code, stdout, stderr := invoke(t, []string{"scan", t.TempDir(), "--workspace-label", "adaptive-ai-orchestrator", "--fail-on", "never"})
		if code != exitOK || stderr != "" || !strings.Contains(stdout, "Workspace: adaptive-ai-orchestrator (explicit label)") || !strings.Contains(stdout, "Output: human-readable text · use --format json for complete safe structured data") || strings.Count(stdout, "\nTarget: .\n") != 1 || !strings.Contains(stdout, "Claude Code\n  Result: No applicable instructions") || !strings.Contains(stdout, "Provider: Anthropic · ID: anthropic-claude-code/cli") || !strings.Contains(stdout, "Scope: supported instruction files only; README, source, and product state excluded") || !strings.Contains(stdout, "Observation: user context not scanned · runtime/model compliance not observed") || !strings.Contains(stdout, "Why: Checked") || !strings.Contains(stdout, "Next steps:") || !strings.Contains(stdout, "Not shown because every selected agent has no applicable instructions") || !strings.Contains(stdout, "\nFindings\n") || strings.Contains(stdout, "Project root:") || strings.Contains(stdout, "Instructions: none") || strings.Contains(stdout, "Scope note:") || strings.Contains(stdout, "\nResult:") || strings.Contains(stdout, "Sensitive user context:") {
			t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
	})
	t.Run("unlabeled text explains hidden workspace", func(t *testing.T) {
		code, stdout, stderr := invoke(t, []string{"explain", t.TempDir(), "--provider", "claude", "--fail-on", "never"})
		if code != exitOK || stderr != "" || !strings.Contains(stdout, "Workspace: hidden (use --workspace-label to add an explicit label)") || strings.Contains(stdout, "<workspace>") {
			t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
	})
	t.Run("workspace label appears in JSON without revealing the path", func(t *testing.T) {
		root := t.TempDir()
		code, stdout, stderr := invoke(t, []string{"scan", root, "--provider", "codex", "--workspace-label", "demo-repository", "--format", "json", "--fail-on", "never"})
		if code != exitOK || stderr != "" || !strings.Contains(stdout, `"workspace_label": "demo-repository"`) || !strings.Contains(stdout, `"code": "ACI001"`) || strings.Contains(stdout, root) {
			t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
	})
	t.Run("unsafe workspace label is rejected", func(t *testing.T) {
		code, _, stderr := invoke(t, []string{"scan", t.TempDir(), "--workspace-label", "private/path"})
		if code != exitUsage || !strings.Contains(stderr, "--workspace-label must be") {
			t.Fatalf("code = %d, stderr = %q", code, stderr)
		}
	})
	t.Run("workspace label is not silently discarded by SARIF", func(t *testing.T) {
		code, _, stderr := invoke(t, []string{"scan", t.TempDir(), "--workspace-label", "demo", "--format", "sarif"})
		if code != exitUsage || !strings.Contains(stderr, "only for text or JSON") {
			t.Fatalf("code = %d, stderr = %q", code, stderr)
		}
	})
	t.Run("provider predictions distinguish effective and mixed results", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Run tests"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := invoke(t, []string{"explain", root, "--provider", "codex"})
		if code != exitOK || stderr != "" || !strings.Contains(stdout, "Result: Applicable instructions found") || !strings.Contains(stdout, "Instructions, in resolution order:") || strings.Contains(stdout, "Why: Checked") || strings.Contains(stdout, "\nResult:") {
			t.Fatalf("effective code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
		code, stdout, stderr = invoke(t, []string{"scan", root, "--fail-on", "never"})
		if code != exitOK || stderr != "" || !strings.Contains(stdout, "Result: Applicable instructions found") || !strings.Contains(stdout, "Result: No applicable instructions") || strings.Contains(stdout, "\nResult:") {
			t.Fatalf("mixed code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
	})
	t.Run("text report makes an empty findings section explicit", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("Run tests"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := invoke(t, []string{"explain", root, "--provider", "claude"})
		if code != exitOK || stderr != "" || !strings.HasSuffix(strings.TrimSpace(stdout), "Findings\n  None.") {
			t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
	})
	t.Run("warning threshold", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Codex only"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, _, _ := invoke(t, []string{"scan", root, "--fail-on", "warning"})
		if code != exitFinding {
			t.Fatalf("code = %d", code)
		}
	})
	t.Run("unsupported provider", func(t *testing.T) {
		code, _, stderr := invoke(t, []string{"scan", t.TempDir(), "--provider", "grok"})
		if code != exitUnsupported || !strings.Contains(stderr, "unsupported provider") {
			t.Fatalf("code = %d, stderr = %q", code, stderr)
		}
	})
	t.Run("Kimi provider", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Run tests"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := invoke(t, []string{"explain", root, "--provider", "kimi"})
		if code != exitOK || stderr != "" || !strings.Contains(stdout, "moonshotai-kimi-code/cli") {
			t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
	})
	t.Run("Gemini provider", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "GEMINI.md"), []byte("Run tests"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := invoke(t, []string{"explain", root, "--provider", "gemini"})
		if code != exitOK || stderr != "" || !strings.Contains(stdout, "google-gemini/cli") {
			t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
	})
	t.Run("diff requires two providers", func(t *testing.T) {
		code, _, _ := invoke(t, []string{"diff", t.TempDir(), "--provider", "claude"})
		if code != exitUsage {
			t.Fatalf("code = %d", code)
		}
	})
	t.Run("analysis help succeeds", func(t *testing.T) {
		code, _, _ := invoke(t, []string{"scan", "--help"})
		if code != exitOK {
			t.Fatalf("code = %d", code)
		}
	})
	t.Run("workspace escape is refused", func(t *testing.T) {
		code, _, _ := invoke(t, []string{"scan", t.TempDir(), "--target", "../outside"})
		if code != exitSafety {
			t.Fatalf("code = %d", code)
		}
	})
	t.Run("json output", func(t *testing.T) {
		code, stdout, _ := invoke(t, []string{"explain", t.TempDir(), "--provider", "codex", "--format", "json"})
		if code != exitOK || !strings.Contains(stdout, `"schema_version": 1`) {
			t.Fatalf("code = %d, stdout = %q", code, stdout)
		}
	})
	t.Run("sarif output", func(t *testing.T) {
		code, stdout, stderr := invoke(t, []string{"scan", t.TempDir(), "--format", "sarif", "--fail-on", "never"})
		if code != exitOK || stderr != "" || !strings.Contains(stdout, `"version": "2.1.0"`) {
			t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
	})
	t.Run("unsupported output format", func(t *testing.T) {
		code, _, stderr := invoke(t, []string{"scan", t.TempDir(), "--format", "xml"})
		if code != exitUsage || !strings.Contains(stderr, "text, json, or sarif") {
			t.Fatalf("code = %d, stderr = %q", code, stderr)
		}
	})
	t.Run("pin and verify round trip", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Run tests"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := invoke(t, []string{"pin", root})
		if code != exitOK || stderr != "" {
			t.Fatalf("pin code = %d, stderr = %q", code, stderr)
		}
		lockBytes, err := os.ReadFile(filepath.Join(root, "agent-config-inspector.lock.json"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(lockBytes), "/home/") || strings.Contains(string(lockBytes), "user-instruction") {
			t.Fatalf("unsafe lockfile = %s", lockBytes)
		}
		code, stdout, stderr := invoke(t, []string{"verify", root})
		if code != exitOK || stderr != "" || !strings.Contains(stdout, "Snapshot: verified") {
			t.Fatalf("verify code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
	})
	t.Run("verify detects drift", func(t *testing.T) {
		root := t.TempDir()
		instruction := filepath.Join(root, "AGENTS.md")
		if err := os.WriteFile(instruction, []byte("Run tests"), 0o600); err != nil {
			t.Fatal(err)
		}
		if code, _, stderr := invoke(t, []string{"pin", root}); code != exitOK {
			t.Fatalf("pin code = %d, stderr = %q", code, stderr)
		}
		if err := os.WriteFile(instruction, []byte("Run all tests"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, stdout, _ := invoke(t, []string{"verify", root})
		if code != exitFinding || !strings.Contains(stdout, "ACI063") {
			t.Fatalf("code = %d, stdout = %q", code, stdout)
		}
	})
	t.Run("pin refuses user context", func(t *testing.T) {
		code, _, stderr := invoke(t, []string{"pin", t.TempDir(), "--include-user-context"})
		if code != exitSafety || !strings.Contains(stderr, "must not encode user context") {
			t.Fatalf("code = %d, stderr = %q", code, stderr)
		}
	})
	t.Run("pin refuses output escape", func(t *testing.T) {
		code, _, _ := invoke(t, []string{"pin", t.TempDir(), "--output", "../outside.lock.json"})
		if code != exitSafety {
			t.Fatalf("code = %d", code)
		}
	})
	t.Run("probe defaults to plan only", func(t *testing.T) {
		code, stdout, stderr := invoke(t, []string{"probe", "codex", "--timeout", "30s"})
		if code != exitOK || stderr != "" || !strings.Contains(stdout, "Behavioral probe plan") || !strings.Contains(stdout, "Expected model calls: 1") {
			t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
	})
	t.Run("probe plan supports unavailable provider binary", func(t *testing.T) {
		code, stdout, stderr := invoke(t, []string{"probe", "kimi", "--format", "json"})
		if code != exitOK || stderr != "" || !strings.Contains(stdout, `"kind": "probe-plan"`) || !strings.Contains(stdout, `"binary_available": false`) {
			t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
	})
	t.Run("probe execution requires quota acknowledgement", func(t *testing.T) {
		code, stdout, stderr := invoke(t, []string{"probe", "codex", "--execute"})
		if code != exitSafety || stdout != "" || !strings.Contains(stderr, "Behavioral probe plan") || !strings.Contains(stderr, "--acknowledge-quota") {
			t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
	})
	t.Run("probe rejects unused quota acknowledgement", func(t *testing.T) {
		code, _, stderr := invoke(t, []string{"probe", "codex", "--acknowledge-quota"})
		if code != exitUsage || !strings.Contains(stderr, "only valid with --execute") {
			t.Fatalf("code = %d, stderr = %q", code, stderr)
		}
	})
	t.Run("probe rejects unsupported case", func(t *testing.T) {
		code, _, stderr := invoke(t, []string{"probe", "claude", "--case", "nested-precedence"})
		if code != exitUnsupported || !strings.Contains(stderr, "unsupported probe case") {
			t.Fatalf("code = %d, stderr = %q", code, stderr)
		}
	})
	t.Run("probe keeps the static Copilot adapter unsupported", func(t *testing.T) {
		code, _, stderr := invoke(t, []string{"probe", "copilot"})
		if code != exitUnsupported || !strings.Contains(stderr, "unsupported probe case") {
			t.Fatalf("code = %d, stderr = %q", code, stderr)
		}
	})
	t.Run("probe keeps Grok unsupported", func(t *testing.T) {
		code, _, stderr := invoke(t, []string{"probe", "grok"})
		if code != exitUnsupported || !strings.Contains(stderr, "unsupported provider") {
			t.Fatalf("code = %d, stderr = %q", code, stderr)
		}
	})
	t.Run("skill inventory hides metadata content", func(t *testing.T) {
		root := t.TempDir()
		skillDir := filepath.Join(root, ".claude", "skills", "review")
		if err := os.MkdirAll(skillDir, 0o700); err != nil {
			t.Fatal(err)
		}
		content := "---\ndescription: PRIVATE_DESCRIPTION_MARKER\n---\nPRIVATE_BODY_MARKER\n"
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := invoke(t, []string{"inventory", "skills", root, "--provider", "claude", "--format", "json"})
		if code != exitOK || stderr != "" || !strings.Contains(stdout, `"available_skills"`) || !strings.Contains(stdout, `"name": "review"`) {
			t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
		if strings.Contains(stdout, "PRIVATE_DESCRIPTION_MARKER") || strings.Contains(stdout, "PRIVATE_BODY_MARKER") || strings.Contains(stdout, root) {
			t.Fatalf("inventory leaked content or absolute path: %s", stdout)
		}
	})
	t.Run("skill inventory rejects unsupported provider", func(t *testing.T) {
		code, _, stderr := invoke(t, []string{"inventory", "skills", t.TempDir(), "--provider", "gemini"})
		if code != exitUnsupported || !strings.Contains(stderr, "skill inventory is not supported") {
			t.Fatalf("code = %d, stderr = %q", code, stderr)
		}
	})
	t.Run("skill inventory refuses workspace escape", func(t *testing.T) {
		code, _, stderr := invoke(t, []string{"inventory", "skills", t.TempDir(), "--target", "../outside"})
		if code != exitSafety || stderr == "" {
			t.Fatalf("code = %d, stderr = %q", code, stderr)
		}
	})
	t.Run("agent inventory hides prompts and configuration values", func(t *testing.T) {
		root := t.TempDir()
		agentDir := filepath.Join(root, ".codex", "agents")
		if err := os.MkdirAll(agentDir, 0o700); err != nil {
			t.Fatal(err)
		}
		content := "name = 'reviewer'\ndescription = 'PRIVATE_AGENT_DESCRIPTION'\ndeveloper_instructions = 'PRIVATE_AGENT_PROMPT'\nmodel = 'PRIVATE_MODEL_VALUE'\n"
		if err := os.WriteFile(filepath.Join(agentDir, "reviewer.toml"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := invoke(t, []string{"inventory", "agents", root, "--provider", "codex", "--format", "json"})
		if code != exitOK || stderr != "" || !strings.Contains(stdout, `"available_agents"`) || !strings.Contains(stdout, `"name": "reviewer"`) {
			t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
		for _, secret := range []string{"PRIVATE_AGENT_DESCRIPTION", "PRIVATE_AGENT_PROMPT", "PRIVATE_MODEL_VALUE", root} {
			if strings.Contains(stdout, secret) {
				t.Fatalf("inventory leaked %q: %s", secret, stdout)
			}
		}
	})
	t.Run("agent inventory rejects unsupported provider", func(t *testing.T) {
		code, _, stderr := invoke(t, []string{"inventory", "agents", t.TempDir(), "--provider", "gemini"})
		if code != exitUnsupported || !strings.Contains(stderr, "agent inventory is not supported") {
			t.Fatalf("code = %d, stderr = %q", code, stderr)
		}
	})
	t.Run("MCP inventory hides connection and credential values", func(t *testing.T) {
		root := t.TempDir()
		content := `{"mcpServers":{"private-server":{"type":"http","url":"https://PRIVATE_MCP_URL.invalid","headers":{"PRIVATE_HEADER_NAME":"PRIVATE_HEADER_VALUE"}}}}`
		if err := os.WriteFile(filepath.Join(root, ".mcp.json"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := invoke(t, []string{"inventory", "mcp", root, "--provider", "claude", "--format", "json", "--fail-on", "never"})
		if code != exitOK || stderr != "" || !strings.Contains(stdout, `"available_servers"`) || !strings.Contains(stdout, `"name": "private-server"`) {
			t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
		for _, secret := range []string{"PRIVATE_MCP_URL", "PRIVATE_HEADER_NAME", "PRIVATE_HEADER_VALUE", root} {
			if strings.Contains(stdout, secret) {
				t.Fatalf("inventory leaked %q: %s", secret, stdout)
			}
		}
	})
	t.Run("MCP inventory rejects unsupported provider", func(t *testing.T) {
		code, _, stderr := invoke(t, []string{"inventory", "mcp", t.TempDir(), "--provider", "gemini"})
		if code != exitUnsupported || !strings.Contains(stderr, "MCP inventory is not supported") {
			t.Fatalf("code = %d, stderr = %q", code, stderr)
		}
	})
}

func invoke(t *testing.T, args []string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

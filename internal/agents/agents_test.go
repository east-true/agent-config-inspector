package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/east-true/agent-config-inspector/internal/provider/registry"
	"github.com/east-true/agent-config-inspector/internal/workspace"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

func TestInventoryProviderRootsAndRedaction(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, ".claude/agents/review/security.md", "---\nname: security-reviewer\ndescription: CLAUDE_PRIVATE_DESCRIPTION\ntools: Read, Grep\n---\nCLAUDE_PRIVATE_INSTRUCTIONS\n")
	writeAgent(t, root, ".codex/agents/reviewer.toml", "name = 'repository_reviewer'\ndescription = 'CODEX_PRIVATE_DESCRIPTION'\ndeveloper_instructions = 'CODEX_PRIVATE_INSTRUCTIONS'\nmodel = 'PRIVATE_MODEL'\nprivate_secret_key = 'PRIVATE_UNKNOWN_VALUE'\n")
	writeAgent(t, root, ".codex/agents/nested/specialist.toml", "name = 'specialist'\ndescription = 'Specialist'\ndeveloper_instructions = 'Special instructions'\n")

	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.AgentInventoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || report.Privacy.UserContextScanned || len(report.Results) != 2 {
		t.Fatalf("report = %#v", report)
	}
	claude := agentResultFor(t, report, claudeID)
	codex := agentResultFor(t, report, codexID)
	if len(claude.AvailableAgents) != 1 || claude.AvailableAgents[0].Name != "security-reviewer" || !claude.AvailableAgents[0].InstructionsPresent {
		t.Fatalf("Claude agents = %#v", claude.AvailableAgents)
	}
	if len(codex.AvailableAgents) != 2 || codex.AvailableAgents[0].Name != "repository_reviewer" || len(codex.AvailableAgents[0].DeclaredCapabilities) != 1 || codex.AvailableAgents[1].Name != "specialist" {
		t.Fatalf("Codex agents = %#v", codex.AvailableAgents)
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
	for _, secret := range []string{"CLAUDE_PRIVATE_DESCRIPTION", "CLAUDE_PRIVATE_INSTRUCTIONS", "CODEX_PRIVATE_DESCRIPTION", "CODEX_PRIVATE_INSTRUCTIONS", "PRIVATE_MODEL", "private_secret_key", "PRIVATE_UNKNOWN_VALUE", root} {
		if strings.Contains(output, secret) {
			t.Fatalf("inventory leaked %q: %s", secret, output)
		}
	}
}

func TestClaudeInventoryUsesClosestScopeAndSelectedHierarchy(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, ".claude/agents/root.md", claudeAgent("reviewer", "Root reviewer"))
	writeAgent(t, root, "packages/api/.claude/agents/team/review.md", claudeAgent("reviewer", "API reviewer"))
	writeAgent(t, root, "packages/web/.claude/agents/review.md", claudeAgent("web-reviewer", "Web reviewer"))

	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.AgentInventoryOptions{Providers: []string{"claude"}, Targets: []string{"packages/api/main.go"}})
	if err != nil {
		t.Fatal(err)
	}
	result := agentResultFor(t, report, claudeID)
	if !report.Complete || len(result.AvailableAgents) != 1 || result.AvailableAgents[0].DisplayPath != "packages/api/.claude/agents/team/review.md" {
		t.Fatalf("result = %#v", result)
	}
	if len(result.ExcludedAgents) != 1 || result.ExcludedAgents[0].DisplayPath != ".claude/agents/root.md" || !hasFinding(result.Findings, "ACI082", "info") {
		t.Fatalf("result = %#v", result)
	}
}

func TestClaudeInventoryMarksSameScopeDuplicateAmbiguous(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, ".claude/agents/one/review.md", claudeAgent("reviewer", "First"))
	writeAgent(t, root, ".claude/agents/two/review.md", claudeAgent("reviewer", "Second"))

	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.AgentInventoryOptions{Providers: []string{"claude"}})
	if err != nil {
		t.Fatal(err)
	}
	result := agentResultFor(t, report, claudeID)
	if report.Complete || result.State != "partial" || len(result.AvailableAgents) != 2 || !hasFinding(result.Findings, "ACI082", "warning") {
		t.Fatalf("result = %#v", result)
	}
	for _, record := range result.AvailableAgents {
		if record.MetadataStatus != "ambiguous" {
			t.Fatalf("record = %#v", record)
		}
	}
}

func TestCodexInventoryRequiresDeveloperInstructions(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, ".codex/agents/reviewer.toml", "name = 'reviewer'\ndescription = 'Review changes'\n")
	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.AgentInventoryOptions{Providers: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	result := agentResultFor(t, report, codexID)
	if len(result.AvailableAgents) != 0 || len(result.ExcludedAgents) != 1 || !hasFinding(result.Findings, "ACI080", "error") {
		t.Fatalf("result = %#v", result)
	}
}

func TestAgentInventoryRejectsUnsafeNameWithoutEchoingIt(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, ".codex/agents/reviewer.toml", "name = \"unsafe\\u001bname\"\ndescription = 'Review changes'\ndeveloper_instructions = 'Review safely'\n")
	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.AgentInventoryOptions{Providers: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	result := agentResultFor(t, report, codexID)
	if len(result.AvailableAgents) != 0 || len(result.ExcludedAgents) != 1 || result.ExcludedAgents[0].Name != "reviewer" || !hasFinding(result.Findings, "ACI081", "error") {
		t.Fatalf("result = %#v", result)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte{0x1b}) {
		t.Fatalf("unsafe control byte leaked: %q", encoded)
	}
}

func TestCodexInventorySelectsFirstSortedSameLayerDuplicate(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, ".codex/agents/a.toml", "name = 'reviewer'\ndescription = 'First'\ndeveloper_instructions = 'First instructions'\n")
	writeAgent(t, root, ".codex/agents/nested/z.toml", "name = 'reviewer'\ndescription = 'Second'\ndeveloper_instructions = 'Second instructions'\n")
	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.AgentInventoryOptions{Providers: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	result := agentResultFor(t, report, codexID)
	if !report.Complete || len(result.AvailableAgents) != 1 || result.AvailableAgents[0].DisplayPath != ".codex/agents/a.toml" || len(result.ExcludedAgents) != 1 || !hasFinding(result.Findings, "ACI082", "warning") {
		t.Fatalf("result = %#v", result)
	}
}

func TestAgentInventoryMarksOversizedSourceIncomplete(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, ".claude/agents/reviewer.md", claudeAgent("reviewer", "Review changes"))
	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.AgentInventoryOptions{Providers: []string{"claude"}, MaxSourceBytes: 16})
	if err != nil {
		t.Fatal(err)
	}
	result := agentResultFor(t, report, claudeID)
	if report.Complete || result.State != "partial" || len(result.ExcludedAgents) != 1 || !hasFinding(result.Findings, "ACI084", "error") {
		t.Fatalf("report = %#v", report)
	}
}

func TestAgentInventorySymlinkPolicy(t *testing.T) {
	root := t.TempDir()
	writeAgent(t, root, "shared/reviewer.md", claudeAgent("reviewer", "Review changes"))
	if err := os.MkdirAll(filepath.Join(root, ".claude", "agents"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "shared", "reviewer.md"), filepath.Join(root, ".claude", "agents", "reviewer.md")); err != nil {
		t.Fatal(err)
	}
	service := New(registry.Builtin())
	report, err := service.Inventory(context.Background(), root, agentconfig.AgentInventoryOptions{Providers: []string{"claude"}})
	if err != nil {
		t.Fatal(err)
	}
	result := agentResultFor(t, report, claudeID)
	if !report.Complete || len(result.AvailableAgents) != 0 || len(result.ExcludedAgents) != 1 || !hasFinding(result.Findings, "ACI083", "info") {
		t.Fatalf("safe result = %#v", result)
	}
	report, err = service.Inventory(context.Background(), root, agentconfig.AgentInventoryOptions{Providers: []string{"claude"}, FollowSymlinks: true})
	if err != nil {
		t.Fatal(err)
	}
	if result = agentResultFor(t, report, claudeID); len(result.AvailableAgents) != 1 || len(result.ExcludedAgents) != 0 {
		t.Fatalf("followed result = %#v", result)
	}

	outside := t.TempDir()
	writeAgent(t, outside, "escape.md", claudeAgent("escape", "Outside"))
	if err := os.Symlink(filepath.Join(outside, "escape.md"), filepath.Join(root, ".claude", "agents", "escape.md")); err != nil {
		t.Fatal(err)
	}
	_, err = service.Inventory(context.Background(), root, agentconfig.AgentInventoryOptions{Providers: []string{"claude"}, FollowSymlinks: true})
	if !errors.Is(err, workspace.ErrOutsideWorkspace) {
		t.Fatalf("err = %v", err)
	}
}

func TestAgentInventoryEnforcesFileCountBound(t *testing.T) {
	root := t.TempDir()
	for index := 0; index <= maxAgentFiles; index++ {
		name := fmt.Sprintf("agent_%03d", index)
		writeAgent(t, root, ".codex/agents/"+name+".toml", "name = '"+name+"'\ndescription = 'Bounded agent'\ndeveloper_instructions = 'Stay bounded'\n")
	}
	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.AgentInventoryOptions{Providers: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	result := agentResultFor(t, report, codexID)
	if report.Complete || result.State != "partial" || len(result.AvailableAgents) != maxAgentFiles || !hasFinding(result.Findings, "ACI084", "error") {
		t.Fatalf("available = %d, state = %s, findings = %#v", len(result.AvailableAgents), result.State, result.Findings)
	}
}

func agentResultFor(t *testing.T, report agentconfig.AgentInventoryReport, providerID string) agentconfig.AgentInventoryResolution {
	t.Helper()
	for _, result := range report.Results {
		if result.Provider.ID == providerID {
			return result
		}
	}
	t.Fatalf("provider %s not found in %#v", providerID, report.Results)
	return agentconfig.AgentInventoryResolution{}
}

func hasFinding(findings []agentconfig.Finding, code, severity string) bool {
	for _, finding := range findings {
		if finding.Code == code && finding.Severity == severity {
			return true
		}
	}
	return false
}

func claudeAgent(name, description string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\nReview the repository.\n"
}

func writeAgent(t *testing.T, root, logical, content string) {
	t.Helper()
	name := filepath.Join(root, filepath.FromSlash(logical))
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

package skills

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

func TestInventoryProviderSpecificRootsAndRedaction(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, ".claude/skills/review/SKILL.md", "---\ndescription: CLAUDE_PRIVATE_DESCRIPTION\n---\nCLAUDE_PRIVATE_BODY\n")
	writeSkill(t, root, ".agents/skills/review/SKILL.md", "---\nname: review\ndescription: >\n  CODEX_PRIVATE_DESCRIPTION\n---\nCODEX_PRIVATE_BODY\n")

	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.SkillInventoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || report.Privacy.UserContextScanned || len(report.Results) != 2 {
		t.Fatalf("report = %#v", report)
	}
	claude := resultFor(t, report, claudeID)
	codex := resultFor(t, report, codexID)
	if len(claude.AvailableSkills) != 1 || claude.AvailableSkills[0].Name != "review" || claude.AvailableSkills[0].DeclaredName != "" {
		t.Fatalf("Claude skills = %#v", claude.AvailableSkills)
	}
	if len(codex.AvailableSkills) != 1 || codex.AvailableSkills[0].Name != "review" || codex.AvailableSkills[0].DescriptionDigest == nil {
		t.Fatalf("Codex skills = %#v", codex.AvailableSkills)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	output := string(encoded)
	var textOutput bytes.Buffer
	if err := WriteText(&textOutput, report); err != nil {
		t.Fatal(err)
	}
	output += textOutput.String()
	for _, secret := range []string{"CLAUDE_PRIVATE_DESCRIPTION", "CLAUDE_PRIVATE_BODY", "CODEX_PRIVATE_DESCRIPTION", "CODEX_PRIVATE_BODY", root} {
		if strings.Contains(output, secret) {
			t.Fatalf("JSON leaked %q: %s", secret, output)
		}
	}
}

func TestInventoryUsesSelectedTargetHierarchy(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, ".agents/skills/root/SKILL.md", "---\nname: root\ndescription: Root skill\n---\n")
	writeSkill(t, root, "packages/api/.agents/skills/api/SKILL.md", "---\nname: api\ndescription: API skill\n---\n")
	writeSkill(t, root, "packages/web/.agents/skills/web/SKILL.md", "---\nname: web\ndescription: Web skill\n---\n")

	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.SkillInventoryOptions{Providers: []string{"codex"}, Targets: []string{"packages/api/main.go"}})
	if err != nil {
		t.Fatal(err)
	}
	result := resultFor(t, report, codexID)
	if len(result.AvailableSkills) != 2 || result.AvailableSkills[0].Name != "api" || result.AvailableSkills[1].Name != "root" {
		t.Fatalf("skills = %#v", result.AvailableSkills)
	}
	for _, skill := range result.AvailableSkills {
		if skill.Name == "web" {
			t.Fatalf("unrelated skill was included: %#v", skill)
		}
	}
}

func TestInventoryKeepsClaudeMalformedMetadataButExcludesCodex(t *testing.T) {
	root := t.TempDir()
	malformed := "---\ndescription: Use when: reviewing\n---\nBody\n"
	writeSkill(t, root, ".claude/skills/review/SKILL.md", malformed)
	writeSkill(t, root, ".agents/skills/review/SKILL.md", "---\nname: review\n---\nBody\n")

	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.SkillInventoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	claude := resultFor(t, report, claudeID)
	codex := resultFor(t, report, codexID)
	if len(claude.AvailableSkills) != 1 || claude.AvailableSkills[0].MetadataStatus != "invalid" {
		t.Fatalf("Claude skills = %#v", claude.AvailableSkills)
	}
	if len(codex.AvailableSkills) != 0 || len(codex.ExcludedSkills) != 1 || codex.ExcludedSkills[0].MetadataStatus != "invalid" {
		t.Fatalf("Codex result = %#v", codex)
	}
	if len(report.Findings) != 2 {
		t.Fatalf("findings = %#v", report.Findings)
	}
}

func TestInventoryReportsDuplicateNamesWithoutSelectingOne(t *testing.T) {
	root := t.TempDir()
	content := "---\nname: review\ndescription: Review changes\n---\n"
	writeSkill(t, root, ".agents/skills/review/SKILL.md", content)
	writeSkill(t, root, "packages/api/.agents/skills/review/SKILL.md", content)

	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.SkillInventoryOptions{Providers: []string{"codex"}, Targets: []string{"packages/api/main.go"}})
	if err != nil {
		t.Fatal(err)
	}
	result := resultFor(t, report, codexID)
	if len(result.AvailableSkills) != 2 || len(result.Findings) != 1 || result.Findings[0].Code != "ACI072" {
		t.Fatalf("result = %#v", result)
	}
}

func TestInventoryMarksOversizedSourceIncomplete(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, ".agents/skills/review/SKILL.md", "---\nname: review\ndescription: Review changes\n---\n")

	report, err := New(registry.Builtin()).Inventory(context.Background(), root, agentconfig.SkillInventoryOptions{Providers: []string{"codex"}, MaxSourceBytes: 16})
	if err != nil {
		t.Fatal(err)
	}
	result := resultFor(t, report, codexID)
	if report.Complete || result.State != "partial" || len(result.ExcludedSkills) != 1 || len(result.Findings) != 1 || result.Findings[0].Code != "ACI074" {
		t.Fatalf("report = %#v", report)
	}
}

func TestInventorySymlinkPolicy(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "shared-skill/SKILL.md", "---\nname: linked\ndescription: Linked skill\n---\n")
	if err := os.MkdirAll(filepath.Join(root, ".agents", "skills"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "shared-skill"), filepath.Join(root, ".agents", "skills", "linked")); err != nil {
		t.Fatal(err)
	}
	service := New(registry.Builtin())
	report, err := service.Inventory(context.Background(), root, agentconfig.SkillInventoryOptions{Providers: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	result := resultFor(t, report, codexID)
	if len(result.AvailableSkills) != 0 || len(result.ExcludedSkills) != 1 || result.Findings[0].Code != "ACI073" {
		t.Fatalf("safe result = %#v", result)
	}
	report, err = service.Inventory(context.Background(), root, agentconfig.SkillInventoryOptions{Providers: []string{"codex"}, FollowSymlinks: true})
	if err != nil {
		t.Fatal(err)
	}
	result = resultFor(t, report, codexID)
	if len(result.AvailableSkills) != 1 || len(result.ExcludedSkills) != 0 {
		t.Fatalf("followed result = %#v", result)
	}

	outside := t.TempDir()
	writeSkill(t, outside, "SKILL.md", "---\nname: escape\ndescription: Escape skill\n---\n")
	if err := os.Symlink(outside, filepath.Join(root, ".agents", "skills", "escape")); err != nil {
		t.Fatal(err)
	}
	_, err = service.Inventory(context.Background(), root, agentconfig.SkillInventoryOptions{Providers: []string{"codex"}, FollowSymlinks: true})
	if !errors.Is(err, workspace.ErrOutsideWorkspace) {
		t.Fatalf("err = %v", err)
	}
}

func resultFor(t *testing.T, report agentconfig.SkillInventoryReport, providerID string) agentconfig.SkillInventoryResolution {
	t.Helper()
	for _, result := range report.Results {
		if result.Provider.ID == providerID {
			return result
		}
	}
	t.Fatalf("provider %s not found in %#v", providerID, report.Results)
	return agentconfig.SkillInventoryResolution{}
}

func writeSkill(t *testing.T, root, logical, content string) {
	t.Helper()
	name := filepath.Join(root, filepath.FromSlash(logical))
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

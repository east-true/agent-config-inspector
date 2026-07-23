package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/east-true/agent-config-inspector/internal/provider/registry"
	"github.com/east-true/agent-config-inspector/internal/workspace"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

const (
	claudeID = "anthropic-claude-code/cli"
	codexID  = "openai-codex/cli"
)

var standardName = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type UnsupportedError struct{ Provider string }

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("skill inventory is not supported for provider %q", e.Provider)
}

type Service struct{ registry *registry.Registry }

func New(providerRegistry *registry.Registry) *Service { return &Service{registry: providerRegistry} }

func (s *Service) Inventory(_ context.Context, workspacePath string, options agentconfig.SkillInventoryOptions) (agentconfig.SkillInventoryReport, error) {
	if workspacePath == "" {
		workspacePath = "."
	}
	view, err := workspace.New(workspacePath, options.MaxSourceBytes, options.FollowSymlinks)
	if err != nil {
		return agentconfig.SkillInventoryReport{}, err
	}
	targets := canonical(options.Targets)
	if len(targets) == 0 {
		targets = []string{"."}
	}
	normalizedTargets := make([]string, 0, len(targets))
	for _, target := range targets {
		normalized, normalizeErr := view.NormalizeTarget(target)
		if normalizeErr != nil {
			return agentconfig.SkillInventoryReport{}, normalizeErr
		}
		normalizedTargets = append(normalizedTargets, normalized)
	}
	targets = canonical(normalizedTargets)
	requested := options.Providers
	if len(requested) == 0 {
		requested = []string{"claude", "codex"}
	}
	identities := make([]agentconfig.ProviderIdentity, 0, len(requested))
	seen := make(map[string]struct{})
	for _, requestedID := range requested {
		adapter, getErr := s.registry.Get(requestedID)
		if getErr != nil {
			return agentconfig.SkillInventoryReport{}, getErr
		}
		identity := adapter.Identity()
		if identity.ID != claudeID && identity.ID != codexID {
			return agentconfig.SkillInventoryReport{}, &UnsupportedError{Provider: requestedID}
		}
		if _, exists := seen[identity.ID]; exists {
			continue
		}
		seen[identity.ID] = struct{}{}
		identities = append(identities, identity)
	}
	sort.Slice(identities, func(i, j int) bool { return identities[i].ID < identities[j].ID })
	providerIDs := make([]string, len(identities))
	for index, identity := range identities {
		providerIDs[index] = identity.ID
	}

	report := agentconfig.SkillInventoryReport{
		SchemaVersion: agentconfig.SkillInventorySchemaVersion,
		Tool:          agentconfig.ToolInfo{Name: "agent-config-inspector", Version: agentconfig.Version, AdapterRegistry: agentconfig.AdapterRegistryVersion},
		Request:       agentconfig.SkillInventoryRequest{Workspace: "<workspace>", Targets: targets, Providers: providerIDs},
		Privacy:       agentconfig.PrivacyInfo{Redaction: "safe", UserContextScanned: false, SensitiveOutput: false},
		Results:       []agentconfig.SkillInventoryResolution{}, Findings: []agentconfig.Finding{}, Complete: true,
	}
	for _, target := range targets {
		for _, identity := range identities {
			result, resultErr := inventoryProvider(view, identity, target)
			if resultErr != nil {
				return agentconfig.SkillInventoryReport{}, resultErr
			}
			if result.State != "complete" {
				report.Complete = false
			}
			report.Results = append(report.Results, result)
			report.Findings = append(report.Findings, result.Findings...)
		}
	}
	sortFindings(report.Findings)
	return report, nil
}

func inventoryProvider(view *workspace.View, identity agentconfig.ProviderIdentity, target string) (agentconfig.SkillInventoryResolution, error) {
	rootName := ".agents/skills"
	evidence := codexEvidence()
	if identity.ID == claudeID {
		rootName = ".claude/skills"
		evidence = claudeEvidence()
	}
	result := agentconfig.SkillInventoryResolution{
		Provider: identity, Capability: "repository-skills-inventory", Target: target, ProjectRoot: view.ProjectRootDisplay(), State: "complete",
		AvailableSkills: []agentconfig.SkillRecord{}, ExcludedSkills: []agentconfig.SkillRecord{}, Evidence: evidence, Findings: []agentconfig.Finding{},
	}
	directories, err := view.DirectoriesToTarget(target)
	if err != nil {
		return result, err
	}
	for _, directory := range directories {
		skillRoot := workspace.Join(directory, rootName)
		children, listErr := view.ChildDirectories(skillRoot)
		if errors.Is(listErr, workspace.ErrSymlink) {
			return result, listErr
		}
		if listErr != nil {
			return result, fmt.Errorf("list skill directory %s: %w", skillRoot, listErr)
		}
		for _, child := range children {
			skillPath := workspace.Join(child.Logical, "SKILL.md")
			if !child.Accessible {
				record := agentconfig.SkillRecord{Name: path.Base(child.Logical), DisplayPath: skillPath, ScopeBase: directory, MetadataStatus: "unread", Reason: "symlinked skill directory is ignored by the safe default"}
				result.ExcludedSkills = append(result.ExcludedSkills, record)
				result.Findings = append(result.Findings, finding("ACI073", "info", "Symlinked skill was not followed", record.Reason, identity.ID, target, skillPath))
				continue
			}
			exists, existsErr := view.Exists(skillPath)
			if existsErr != nil {
				return result, existsErr
			}
			if !exists {
				continue
			}
			file, readErr := view.Read(skillPath)
			if readErr != nil {
				reason := readErr.Error()
				record := agentconfig.SkillRecord{Name: path.Base(child.Logical), DisplayPath: skillPath, ScopeBase: directory, MetadataStatus: "unread", SourceBytes: file.Size, Reason: reason}
				result.ExcludedSkills = append(result.ExcludedSkills, record)
				result.Findings = append(result.Findings, finding("ACI074", "error", "Skill source could not be inspected", reason, identity.ID, target, skillPath))
				result.State = "partial"
				continue
			}
			record, available, recordFindings := inspectSkill(identity.ID, target, directory, child.Logical, file)
			result.Findings = append(result.Findings, recordFindings...)
			if available {
				result.AvailableSkills = append(result.AvailableSkills, record)
			} else {
				result.ExcludedSkills = append(result.ExcludedSkills, record)
			}
		}
	}
	sort.Slice(result.AvailableSkills, func(i, j int) bool { return skillLess(result.AvailableSkills[i], result.AvailableSkills[j]) })
	sort.Slice(result.ExcludedSkills, func(i, j int) bool { return skillLess(result.ExcludedSkills[i], result.ExcludedSkills[j]) })
	result.Findings = append(result.Findings, duplicateFindings(identity.ID, target, result.AvailableSkills)...)
	sortFindings(result.Findings)
	return result, nil
}

func inspectSkill(providerID, target, scopeBase, directory string, file workspace.File) (agentconfig.SkillRecord, bool, []agentconfig.Finding) {
	directoryName := path.Base(directory)
	record := agentconfig.SkillRecord{
		Name: directoryName, DisplayPath: file.Logical, ScopeBase: scopeBase,
		SourceDigest: digest("agent-config-inspector/skill-source/v1", file.Bytes), SourceBytes: file.Size,
		MetadataStatus: "valid", Reason: "repository skill is discoverable for the selected target",
	}
	parsed, parseErr := parseMetadata(file.Bytes)
	if parseErr != nil {
		record.MetadataStatus = "invalid"
		record.Reason = parseErr.Error()
		severity := "warning"
		available := providerID == claudeID
		if !available {
			severity = "error"
		}
		return record, available, []agentconfig.Finding{finding("ACI070", severity, "Skill metadata is invalid", record.Reason, providerID, target, file.Logical)}
	}
	if providerID == claudeID && parsed.hasName {
		record.DeclaredName = parsed.name
	}
	if parsed.hasDescription && strings.TrimSpace(parsed.description) != "" {
		record.DescriptionPresent = true
		record.DescriptionBytes = len([]byte(parsed.description))
		record.DescriptionDigest = digest("agent-config-inspector/skill-description/v1", []byte(parsed.description))
	}

	var findings []agentconfig.Finding
	if providerID == codexID {
		record.Name = parsed.name
		if !parsed.hasName || parsed.name == "" {
			record.Name = directoryName
			record.MetadataStatus = "invalid"
			record.Reason = "Codex requires a non-empty name field"
			return record, false, []agentconfig.Finding{finding("ACI070", "error", "Skill metadata is incomplete", record.Reason, providerID, target, file.Logical)}
		}
		if !parsed.hasDescription || strings.TrimSpace(parsed.description) == "" {
			record.MetadataStatus = "invalid"
			record.Reason = "Codex requires a non-empty description field"
			return record, false, []agentconfig.Finding{finding("ACI070", "error", "Skill metadata is incomplete", record.Reason, providerID, target, file.Logical)}
		}
		if len(parsed.name) > 64 || !standardName.MatchString(parsed.name) || parsed.name != directoryName {
			record.MetadataStatus = "invalid"
			record.Reason = "Codex skill name must match its directory and use the Agent Skills name format"
			return record, false, []agentconfig.Finding{finding("ACI071", "error", "Skill name violates the Codex contract", record.Reason, providerID, target, file.Logical)}
		}
		if utf8.RuneCountInString(parsed.description) > 1024 {
			record.MetadataStatus = "invalid"
			record.Reason = "Codex skill description exceeds the 1024-character Agent Skills limit"
			return record, false, []agentconfig.Finding{finding("ACI070", "error", "Skill description is too large", record.Reason, providerID, target, file.Logical)}
		}
		return record, true, findings
	}

	// Claude Code derives the invocation name from the project skill directory.
	// Its current CLI accepts missing metadata, but a missing description makes
	// automatic selection unreliable.
	if !record.DescriptionPresent {
		record.MetadataStatus = "partial"
		record.Reason = "Claude can invoke this skill by directory name, but automatic matching lacks a description"
		findings = append(findings, finding("ACI070", "warning", "Claude skill has no usable description", record.Reason, providerID, target, file.Logical))
	}
	if parsed.hasName && parsed.name != "" && parsed.name != directoryName {
		findings = append(findings, finding("ACI071", "info", "Claude skill display name differs from its invocation name", "Claude project skills are invoked by directory name; frontmatter name is only a display label.", providerID, target, file.Logical))
	}
	return record, true, findings
}

func duplicateFindings(providerID, target string, records []agentconfig.SkillRecord) []agentconfig.Finding {
	pathsByName := make(map[string][]string)
	for _, record := range records {
		pathsByName[record.Name] = append(pathsByName[record.Name], record.DisplayPath)
	}
	var findings []agentconfig.Finding
	for name, paths := range pathsByName {
		if len(paths) < 2 {
			continue
		}
		sort.Strings(paths)
		findings = append(findings, agentconfig.Finding{
			Code: "ACI072", Severity: "info", Title: "Multiple discoverable skills share a name",
			Summary:   fmt.Sprintf("%d repository skills resolve to the name %q; the inventory preserves every source without predicting runtime selection.", len(paths), name),
			Providers: []string{providerID}, Targets: []string{target}, Sources: paths, Confidence: "high",
		})
	}
	return findings
}

func finding(code, severity, title, summary, providerID, target, source string) agentconfig.Finding {
	return agentconfig.Finding{Code: code, Severity: severity, Title: title, Summary: summary, Providers: []string{providerID}, Targets: []string{target}, Sources: []string{source}, Confidence: "high"}
}

func digest(domain string, value []byte) *agentconfig.Digest {
	hash := sha256.New()
	hash.Write([]byte(domain))
	hash.Write([]byte{0})
	hash.Write(value)
	return &agentconfig.Digest{Algorithm: "sha256", Value: hex.EncodeToString(hash.Sum(nil))}
}

func skillLess(first, second agentconfig.SkillRecord) bool {
	if first.Name != second.Name {
		return first.Name < second.Name
	}
	return first.DisplayPath < second.DisplayPath
}

func canonical(values []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, value := range values {
		if value == "" {
			value = "."
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sortFindings(findings []agentconfig.Finding) {
	severity := map[string]int{"error": 0, "warning": 1, "info": 2}
	sort.SliceStable(findings, func(i, j int) bool {
		if severity[findings[i].Severity] != severity[findings[j].Severity] {
			return severity[findings[i].Severity] < severity[findings[j].Severity]
		}
		if findings[i].Code != findings[j].Code {
			return findings[i].Code < findings[j].Code
		}
		return findings[i].Summary < findings[j].Summary
	})
}

func claudeEvidence() []agentconfig.EvidenceRecord {
	return []agentconfig.EvidenceRecord{{
		ID: "claude-code-skills-2026-07-24", Kind: "official-doc", Claim: "Claude Code discovers project skills from .claude/skills on the selected path hierarchy.",
		Provider: "Anthropic", Surface: "Claude Code", VersionRange: ">=2.1.203", SourceURL: "https://code.claude.com/docs/en/slash-commands", CheckedOn: "2026-07-24", Status: "documented",
	}}
}

func codexEvidence() []agentconfig.EvidenceRecord {
	return []agentconfig.EvidenceRecord{{
		ID: "codex-skills-2026-07-24", Kind: "official-doc", Claim: "Codex discovers repository skills from .agents/skills between the launch directory and repository root.",
		Provider: "OpenAI", Surface: "Codex CLI", VersionRange: "current", SourceURL: "https://learn.chatgpt.com/docs/build-skills.md", CheckedOn: "2026-07-24", Status: "documented",
	}}
}

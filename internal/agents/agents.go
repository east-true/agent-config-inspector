package agents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/east-true/agent-config-inspector/internal/provider/registry"
	"github.com/east-true/agent-config-inspector/internal/workspace"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

const (
	claudeID      = "anthropic-claude-code/cli"
	codexID       = "openai-codex/cli"
	maxAgentFiles = 512
)

var claudeAgentName = regexp.MustCompile(`^[a-z]+(?:-[a-z]+)*$`)

type UnsupportedError struct{ Provider string }

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("agent inventory is not supported for provider %q", e.Provider)
}

type Service struct{ registry *registry.Registry }

func New(providerRegistry *registry.Registry) *Service { return &Service{registry: providerRegistry} }

func (s *Service) Inventory(_ context.Context, workspacePath string, options agentconfig.AgentInventoryOptions) (agentconfig.AgentInventoryReport, error) {
	if workspacePath == "" {
		workspacePath = "."
	}
	view, err := workspace.New(workspacePath, options.MaxSourceBytes, options.FollowSymlinks)
	if err != nil {
		return agentconfig.AgentInventoryReport{}, err
	}
	targets, err := normalizedTargets(view, options.Targets)
	if err != nil {
		return agentconfig.AgentInventoryReport{}, err
	}
	identities, err := s.identities(options.Providers)
	if err != nil {
		return agentconfig.AgentInventoryReport{}, err
	}
	providerIDs := make([]string, len(identities))
	for index, identity := range identities {
		providerIDs[index] = identity.ID
	}
	report := agentconfig.AgentInventoryReport{
		SchemaVersion: agentconfig.AgentInventorySchemaVersion,
		Tool:          agentconfig.ToolInfo{Name: "agent-config-inspector", Version: agentconfig.Version, AdapterRegistry: agentconfig.AdapterRegistryVersion},
		Request:       agentconfig.AgentInventoryRequest{Workspace: "<workspace>", Targets: targets, Providers: providerIDs},
		Privacy:       agentconfig.PrivacyInfo{Redaction: "safe", UserContextScanned: false, SensitiveOutput: false},
		Results:       []agentconfig.AgentInventoryResolution{}, Findings: []agentconfig.Finding{}, Complete: true,
	}
	for _, target := range targets {
		for _, identity := range identities {
			result, resultErr := inventoryProvider(view, identity, target)
			if resultErr != nil {
				return agentconfig.AgentInventoryReport{}, resultErr
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

func (s *Service) identities(requested []string) ([]agentconfig.ProviderIdentity, error) {
	if len(requested) == 0 {
		requested = []string{"claude", "codex"}
	}
	seen := make(map[string]struct{})
	identities := make([]agentconfig.ProviderIdentity, 0, len(requested))
	for _, requestedID := range requested {
		adapter, err := s.registry.Get(requestedID)
		if err != nil {
			return nil, err
		}
		identity := adapter.Identity()
		if identity.ID != claudeID && identity.ID != codexID {
			return nil, &UnsupportedError{Provider: requestedID}
		}
		if _, exists := seen[identity.ID]; exists {
			continue
		}
		seen[identity.ID] = struct{}{}
		identities = append(identities, identity)
	}
	sort.Slice(identities, func(i, j int) bool { return identities[i].ID < identities[j].ID })
	return identities, nil
}

func inventoryProvider(view *workspace.View, identity agentconfig.ProviderIdentity, target string) (agentconfig.AgentInventoryResolution, error) {
	result := agentconfig.AgentInventoryResolution{
		Provider: identity, Capability: "repository-agents-inventory", Target: target, ProjectRoot: view.ProjectRootDisplay(), State: "complete",
		AvailableAgents: []agentconfig.AgentRecord{}, ExcludedAgents: []agentconfig.AgentRecord{}, Evidence: evidence(identity.ID), Findings: []agentconfig.Finding{},
	}
	directories, err := view.DirectoriesToTarget(target)
	if err != nil {
		return result, err
	}
	var candidates []agentconfig.AgentRecord
	fileCount := 0
	limitReported := false
	// Prefer the closest scope if the defensive file cap is reached. Output is
	// sorted later, so discovery order is not exposed as provider precedence.
	for directoryIndex := len(directories) - 1; directoryIndex >= 0; directoryIndex-- {
		directory := directories[directoryIndex]
		rootName := ".codex/agents"
		if identity.ID == claudeID {
			rootName = ".claude/agents"
		}
		agentRoot := workspace.Join(directory, rootName)
		files, walkErr := view.WalkFiles(agentRoot, func(candidate string, _ fs.DirEntry) bool {
			if identity.ID == claudeID {
				return path.Ext(candidate) == ".md"
			}
			return path.Ext(candidate) == ".toml"
		})
		if errors.Is(walkErr, workspace.ErrSymlink) {
			return result, walkErr
		}
		if walkErr != nil {
			return result, fmt.Errorf("list agent directory %s: inventory traversal failed", agentRoot)
		}
		if remaining := maxAgentFiles - fileCount; len(files) > remaining {
			files = files[:max(remaining, 0)]
			if !limitReported {
				result.Findings = append(result.Findings, finding("ACI084", "error", "Agent inventory file limit reached", fmt.Sprintf("At most %d repository agent files are inspected per provider and target.", maxAgentFiles), identity.ID, target, agentRoot))
				result.State = "partial"
				limitReported = true
			}
		}
		fileCount += len(files)
		for _, candidate := range files {
			file, readErr := view.Read(candidate)
			if readErr != nil {
				if errors.Is(readErr, workspace.ErrOutsideWorkspace) {
					return result, readErr
				}
				displayCandidate := candidate
				if errors.Is(readErr, workspace.ErrInvalidPath) {
					displayCandidate = "<invalid-agent-path>"
				}
				reason := "agent source could not be read safely"
				if errors.Is(readErr, workspace.ErrTooLarge) {
					reason = "agent source exceeds the configured byte limit"
				}
				record := agentconfig.AgentRecord{
					Name: path.Base(strings.TrimSuffix(displayCandidate, path.Ext(displayCandidate))), DisplayPath: displayCandidate, ScopeBase: directory,
					Format: agentFormat(identity.ID), SourceBytes: file.Size, MetadataStatus: "unread", Reason: reason,
				}
				result.ExcludedAgents = append(result.ExcludedAgents, record)
				if errors.Is(readErr, workspace.ErrSymlink) {
					record.Reason = "symlinked agent source is ignored by the safe default"
					result.ExcludedAgents[len(result.ExcludedAgents)-1] = record
					result.Findings = append(result.Findings, finding("ACI083", "info", "Symlinked agent source was not followed", record.Reason, identity.ID, target, displayCandidate))
					continue
				}
				result.Findings = append(result.Findings, finding("ACI084", "error", "Agent source could not be inspected", record.Reason, identity.ID, target, displayCandidate))
				result.State = "partial"
				continue
			}
			record, available, recordFindings := inspectAgent(identity.ID, target, directory, file)
			result.Findings = append(result.Findings, recordFindings...)
			if available {
				candidates = append(candidates, record)
			} else {
				result.ExcludedAgents = append(result.ExcludedAgents, record)
			}
		}
	}
	available, shadowed, duplicateFindings, partial := resolveDuplicates(identity.ID, target, candidates)
	result.AvailableAgents = available
	result.ExcludedAgents = append(result.ExcludedAgents, shadowed...)
	result.Findings = append(result.Findings, duplicateFindings...)
	if partial {
		result.State = "partial"
	}
	if identity.ID == codexID {
		result.Findings = append(result.Findings, agentconfig.Finding{
			Code: "ACI021", Severity: "info", Title: "Codex project trust is a runtime condition",
			Summary:   "Static inventory assumes project-scoped Codex configuration is trusted; an untrusted runtime may ignore .codex agent definitions.",
			Providers: []string{identity.ID}, Targets: []string{target}, Confidence: "high",
		})
	}
	sortRecords(result.ExcludedAgents)
	sortFindings(result.Findings)
	return result, nil
}

func agentFormat(providerID string) string {
	if providerID == claudeID {
		return "markdown-frontmatter"
	}
	return "toml"
}

func inspectAgent(providerID, target, scopeBase string, file workspace.File) (agentconfig.AgentRecord, bool, []agentconfig.Finding) {
	format := agentFormat(providerID)
	parsed, parseErr := parseCodexAgent(file.Bytes)
	if providerID == claudeID {
		parsed, parseErr = parseClaudeAgent(file.Bytes)
	}
	record := agentconfig.AgentRecord{
		Name: path.Base(strings.TrimSuffix(file.Logical, path.Ext(file.Logical))), DisplayPath: file.Logical, ScopeBase: scopeBase, Format: format,
		SourceDigest: digest("agent-config-inspector/agent-source/v1", file.Bytes), SourceBytes: file.Size,
		MetadataStatus: "valid", Reason: "repository custom agent is discoverable for the selected target",
	}
	if parseErr != nil {
		record.MetadataStatus = "invalid"
		record.Reason = parseErr.Error()
		return record, false, []agentconfig.Finding{finding("ACI080", "error", "Agent metadata is invalid", record.Reason, providerID, target, file.Logical)}
	}
	declaredName := parsed.name
	if providerID == codexID {
		declaredName = strings.TrimSpace(parsed.name)
	}
	record.DeclaredCapabilities = parsed.declaredCapabilities
	if parsed.hasDescription && strings.TrimSpace(parsed.description) != "" {
		record.DescriptionPresent = true
		record.DescriptionBytes = len([]byte(parsed.description))
		record.DescriptionDigest = digest("agent-config-inspector/agent-description/v1", []byte(parsed.description))
	}
	if parsed.hasInstructions && strings.TrimSpace(parsed.instructions) != "" {
		record.InstructionsPresent = true
		record.InstructionsBytes = len([]byte(parsed.instructions))
		record.InstructionsDigest = digest("agent-config-inspector/agent-instructions/v1", []byte(parsed.instructions))
	}
	if !parsed.hasName || declaredName == "" || !record.DescriptionPresent {
		record.MetadataStatus = "invalid"
		record.Reason = "agent requires non-empty name and description fields"
		return record, false, []agentconfig.Finding{finding("ACI080", "error", "Agent metadata is incomplete", record.Reason, providerID, target, file.Logical)}
	}
	if len([]byte(declaredName)) > 128 || !utf8.ValidString(declaredName) || strings.IndexFunc(declaredName, unicode.IsControl) >= 0 {
		record.MetadataStatus = "invalid"
		record.Reason = "agent name exceeds the safe 128-byte output bound or contains invalid characters"
		return record, false, []agentconfig.Finding{finding("ACI081", "error", "Agent name is unsafe to report", record.Reason, providerID, target, file.Logical)}
	}
	record.Name = declaredName
	if providerID == claudeID && !claudeAgentName.MatchString(declaredName) {
		record.MetadataStatus = "invalid"
		record.Reason = "Claude agent name must use lowercase letters separated by single hyphens"
		return record, false, []agentconfig.Finding{finding("ACI081", "error", "Agent name violates the Claude contract", record.Reason, providerID, target, file.Logical)}
	}
	if providerID == codexID && !record.InstructionsPresent {
		record.MetadataStatus = "invalid"
		record.Reason = "Codex custom agent requires non-empty developer_instructions"
		return record, false, []agentconfig.Finding{finding("ACI080", "error", "Agent metadata is incomplete", record.Reason, providerID, target, file.Logical)}
	}
	return record, true, nil
}

func resolveDuplicates(providerID, target string, records []agentconfig.AgentRecord) ([]agentconfig.AgentRecord, []agentconfig.AgentRecord, []agentconfig.Finding, bool) {
	groups := make(map[string][]agentconfig.AgentRecord)
	for _, record := range records {
		groups[record.Name] = append(groups[record.Name], record)
	}
	available := []agentconfig.AgentRecord{}
	excluded := []agentconfig.AgentRecord{}
	var findings []agentconfig.Finding
	partial := false
	for name, group := range groups {
		if len(group) == 1 {
			available = append(available, group[0])
			continue
		}
		sources := make([]string, len(group))
		for index, record := range group {
			sources[index] = record.DisplayPath
		}
		sort.Strings(sources)
		closestDepth := -1
		for _, record := range group {
			if depth := scopeDepth(record.ScopeBase); depth > closestDepth {
				closestDepth = depth
			}
		}
		var closest []agentconfig.AgentRecord
		for _, record := range group {
			if scopeDepth(record.ScopeBase) == closestDepth {
				closest = append(closest, record)
			} else {
				record.Reason = "a closer project agent layer with the same name takes precedence"
				excluded = append(excluded, record)
			}
		}
		if len(closest) == 1 {
			available = append(available, closest[0])
			findings = append(findings, agentconfig.Finding{Code: "ACI082", Severity: "info", Title: "Closer project agent shadows another definition", Summary: fmt.Sprintf("The closest definition for agent %q is effective for the selected target.", name), Providers: []string{providerID}, Targets: []string{target}, Sources: sources, Confidence: "high"})
			continue
		}
		group = closest
		if providerID == codexID {
			sortRecords(group)
			available = append(available, group[0])
			for _, record := range group[1:] {
				record.Reason = "an earlier lexicographically sorted Codex agent file in the same project layer uses this name"
				excluded = append(excluded, record)
			}
			findings = append(findings, agentconfig.Finding{Code: "ACI082", Severity: "warning", Title: "Codex project layer contains a duplicate agent name", Summary: fmt.Sprintf("Codex keeps the first sorted definition for agent %q in the closest project layer and warns about later duplicates.", name), Providers: []string{providerID}, Targets: []string{target}, Sources: sources, Confidence: "high"})
			continue
		}
		partial = true
		for _, record := range group {
			record.MetadataStatus = "ambiguous"
			record.Reason = "multiple agent definitions share this name and modeled selection is not deterministic"
			available = append(available, record)
		}
		findings = append(findings, agentconfig.Finding{Code: "ACI082", Severity: "warning", Title: "Agent name selection is ambiguous", Summary: fmt.Sprintf("%d repository agent definitions share the name %q without a deterministic modeled winner.", len(group), name), Providers: []string{providerID}, Targets: []string{target}, Sources: sources, Confidence: "high"})
	}
	sortRecords(available)
	sortRecords(excluded)
	return available, excluded, findings, partial
}

func normalizedTargets(view *workspace.View, values []string) ([]string, error) {
	if len(values) == 0 {
		values = []string{"."}
	}
	seen := make(map[string]struct{})
	var result []string
	for _, value := range values {
		if value == "" {
			value = "."
		}
		normalized, err := view.NormalizeTarget(value)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	sort.Strings(result)
	return result, nil
}

func scopeDepth(scope string) int {
	if scope == "." || scope == "" {
		return 0
	}
	return strings.Count(scope, "/") + 1
}

func sortRecords(records []agentconfig.AgentRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].Name != records[j].Name {
			return records[i].Name < records[j].Name
		}
		return records[i].DisplayPath < records[j].DisplayPath
	})
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

func evidence(providerID string) []agentconfig.EvidenceRecord {
	if providerID == claudeID {
		return []agentconfig.EvidenceRecord{{ID: "claude-code-subagents-2026-07-24", Kind: "official-doc", Claim: "Claude Code recursively discovers project agent Markdown files from .claude/agents on the working-directory hierarchy.", Provider: "Anthropic", Surface: "Claude Code", VersionRange: ">=2.1.178", SourceURL: "https://code.claude.com/docs/en/sub-agents", CheckedOn: "2026-07-24", Status: "documented"}}
	}
	return []agentconfig.EvidenceRecord{{ID: "codex-custom-agents-2026-07-24", Kind: "official-source", Claim: "Codex recursively discovers project-scoped agent TOML files from the agents directory of each enabled configuration layer.", Provider: "OpenAI", Surface: "Codex CLI", VersionRange: "current source", SourceURL: "https://github.com/openai/codex/blob/main/codex-rs/core/src/config/agent_roles.rs", CheckedOn: "2026-07-24", Status: "source-verified"}}
}

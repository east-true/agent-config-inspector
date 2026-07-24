package app

import (
	"context"
	"fmt"
	"sort"

	"github.com/east-true/agent-config-inspector/internal/compare"
	"github.com/east-true/agent-config-inspector/internal/provider"
	"github.com/east-true/agent-config-inspector/internal/provider/registry"
	"github.com/east-true/agent-config-inspector/internal/usercontext"
	"github.com/east-true/agent-config-inspector/internal/workspace"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

type Scanner struct {
	Registry *registry.Registry
}

func New() *Scanner { return &Scanner{Registry: registry.Builtin()} }

func (s *Scanner) Scan(ctx context.Context, workspacePath string, options agentconfig.ScanOptions) (agentconfig.Report, error) {
	if workspacePath == "" {
		workspacePath = "."
	}
	view, err := workspace.New(workspacePath, options.MaxSourceBytes, options.FollowSymlinks)
	if err != nil {
		return agentconfig.Report{}, err
	}
	workspaceLabel, err := workspace.NormalizeLabel(options.WorkspaceLabel)
	if err != nil {
		return agentconfig.Report{}, err
	}
	targets := canonicalValues(options.Targets)
	if len(targets) == 0 {
		targets = []string{"."}
	}
	providerIDs := options.Providers
	if len(providerIDs) == 0 {
		providerIDs = s.Registry.DefaultIDs()
	}
	adapters := make([]provider.Adapter, 0, len(providerIDs))
	canonicalProviders := make([]string, 0, len(providerIDs))
	seenProviders := make(map[string]struct{})
	for _, id := range providerIDs {
		adapter, getErr := s.Registry.Get(id)
		if getErr != nil {
			return agentconfig.Report{}, getErr
		}
		canonical := adapter.Identity().ID
		if _, ok := seenProviders[canonical]; ok {
			continue
		}
		seenProviders[canonical] = struct{}{}
		adapters = append(adapters, adapter)
		canonicalProviders = append(canonicalProviders, canonical)
	}
	sort.Slice(adapters, func(i, j int) bool { return adapters[i].Identity().ID < adapters[j].Identity().ID })
	sort.Strings(canonicalProviders)

	report := agentconfig.Report{
		SchemaVersion: agentconfig.SchemaVersion,
		Tool:          agentconfig.ToolInfo{Name: "agent-config-inspector", Version: agentconfig.Version, AdapterRegistry: agentconfig.AdapterRegistryVersion},
		Request:       agentconfig.RequestInfo{Workspace: "<workspace>", WorkspaceLabel: workspaceLabel, Targets: targets, Providers: canonicalProviders},
		Privacy:       agentconfig.PrivacyInfo{Redaction: "safe", UserContextScanned: options.IncludeUserContext, SensitiveOutput: false},
		Results:       []agentconfig.Resolution{},
		Comparisons:   []agentconfig.Comparison{},
		Findings:      []agentconfig.Finding{},
		Complete:      true,
	}
	externalByProvider := make(map[string][]provider.ExternalSource)
	if options.IncludeUserContext {
		for _, adapter := range adapters {
			external, loadErr := usercontext.Load(adapter.Identity().ID, options.MaxSourceBytes)
			if loadErr != nil {
				return agentconfig.Report{}, fmt.Errorf("load redacted user context for %s: %w", adapter.Identity().ID, loadErr)
			}
			externalByProvider[adapter.Identity().ID] = external
		}
	}
	for _, target := range targets {
		for _, adapter := range adapters {
			resolution, resolveErr := adapter.Resolve(ctx, view, target, provider.Options{
				IncludeUserContext: options.IncludeUserContext,
				MaxImportDepth:     options.MaxImportDepth,
				ExternalSources:    externalByProvider[adapter.Identity().ID],
			})
			if resolveErr != nil {
				return agentconfig.Report{}, fmt.Errorf("resolve %s for %s: %w", adapter.Identity().ID, target, resolveErr)
			}
			for _, finding := range resolution.Findings {
				if finding.Code == "ACI004" || finding.Code == "ACI006" || finding.Code == "ACI007" || finding.Code == "ACI026" || finding.Code == "ACI027" {
					resolution.State = "partial"
					report.Complete = false
				}
			}
			if resolution.Prediction == "predicted-empty" && len(resolution.ExcludedSources) == 0 {
				emptyFinding := emptyDiscoveryFinding(resolution.Provider.ID, target)
				resolution.Findings = append(resolution.Findings, emptyFinding)
			}
			compare.SortFindings(resolution.Findings)
			report.Results = append(report.Results, resolution)
			report.Findings = append(report.Findings, resolution.Findings...)
		}
	}
	report.Comparisons, err = compareResults(report.Results)
	if err != nil {
		return agentconfig.Report{}, err
	}
	_, comparisonFindings := compare.All(report.Results)
	report.Findings = append(report.Findings, comparisonFindings...)
	compare.SortFindings(report.Findings)
	if options.IncludeUserContext {
		redactUserContext(&report)
	}
	return report, nil
}

func emptyDiscoveryFinding(providerID, target string) agentconfig.Finding {
	checked := map[string]string{
		"anthropic-claude-code/cli": "CLAUDE.md, CLAUDE.local.md, .claude/CLAUDE.md, and .claude/rules/**/*.md along the selected target hierarchy",
		"github-copilot/cli":        ".github/copilot-instructions.md, compatible AGENTS.md/CLAUDE.md/GEMINI.md files, and .github/instructions/**/*.instructions.md along the selected target hierarchy",
		"google-gemini/cli":         "configured Gemini context filenames (GEMINI.md by default) along the selected target hierarchy",
		"moonshotai-kimi-code/cli":  ".kimi-code/AGENTS.md, AGENTS.md, and agents.md inside the selected Git-root hierarchy",
		"openai-codex/cli":          "AGENTS.override.md, AGENTS.md, and configured fallback instruction filenames along the selected target hierarchy",
	}[providerID]
	if checked == "" {
		checked = "the selected adapter's documented repository instruction locations"
	}
	addGuidance := map[string]string{
		"anthropic-claude-code/cli": "Add CLAUDE.md or another supported Claude Code instruction source.",
		"github-copilot/cli":        "Add .github/copilot-instructions.md or another supported Copilot CLI instruction source.",
		"google-gemini/cli":         "Add GEMINI.md or another configured Gemini CLI context file.",
		"moonshotai-kimi-code/cli":  "Add .kimi-code/AGENTS.md or another supported Kimi instruction source.",
		"openai-codex/cli":          "Add AGENTS.md or another supported Codex instruction source.",
	}[providerID]
	if addGuidance == "" {
		addGuidance = "Add a documented provider instruction file."
	}
	remediation := []string{addGuidance}
	if providerID == "anthropic-claude-code/cli" || providerID == "openai-codex/cli" {
		remediation = append(remediation, "Inspect separate configuration with inventory skills, inventory agents, or inventory mcp.")
	}
	return agentconfig.Finding{
		Code: "ACI001", Severity: "info", Title: "No repository instruction source was discovered",
		Summary:   "Checked " + checked + "; no applicable source was found.",
		Providers: []string{providerID}, Targets: []string{target}, Confidence: "high",
		Remediation: remediation,
	}
}

func redactUserContext(report *agentconfig.Report) {
	targetHasUserContext := make(map[string]bool)
	for resultIndex := range report.Results {
		result := &report.Results[resultIndex]
		hasUser := false
		for sourceIndex := range result.IncludedSources {
			source := &result.IncludedSources[sourceIndex]
			if source.Origin == "user" {
				hasUser = true
				source.SizeBytes = 0
				source.RawDigest = nil
				source.NormalizedDigest = nil
				source.Units = nil
				source.Scope.Patterns = nil
			}
		}
		for sourceIndex := range result.ExcludedSources {
			source := &result.ExcludedSources[sourceIndex]
			if source.Origin == "user" {
				source.SizeBytes = 0
				source.RawDigest = nil
				source.NormalizedDigest = nil
				source.Units = nil
				source.Scope.Patterns = nil
			}
		}
		if hasUser {
			targetHasUserContext[result.Target] = true
			result.EffectiveDigest = agentconfig.Digest{Algorithm: "redacted", Value: "hidden"}
			result.TokenEstimate = agentconfig.TokenEstimate{Method: "redacted", Value: 0}
		}
	}
	for comparisonIndex := range report.Comparisons {
		comparison := &report.Comparisons[comparisonIndex]
		if !targetHasUserContext[comparison.Target] {
			continue
		}
		for index := range comparison.OnlyInFirst {
			comparison.OnlyInFirst[index] = fmt.Sprintf("<redacted-unit-%d>", index+1)
		}
		for index := range comparison.OnlyInSecond {
			comparison.OnlyInSecond[index] = fmt.Sprintf("<redacted-unit-%d>", index+1)
		}
	}
	for findingIndex := range report.Findings {
		finding := &report.Findings[findingIndex]
		if finding.Code == "ACI040" && len(finding.Targets) > 0 && targetHasUserContext[finding.Targets[0]] {
			finding.Summary = "The providers' effective instruction graphs differ; unit details are redacted because user context was included."
		}
	}
}

func compareResults(results []agentconfig.Resolution) ([]agentconfig.Comparison, error) {
	comparisons, _ := compare.All(results)
	return comparisons, nil
}

func canonicalValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			value = "."
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

package kimi

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/east-true/agent-config-inspector/internal/provider"
	"github.com/east-true/agent-config-inspector/internal/workspace"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

const (
	providerID             = "moonshotai-kimi-code/cli"
	recommendedMaxBytes    = 32 * 1024
	instructionEvidenceURL = "https://github.com/MoonshotAI/kimi-code/blob/%40moonshot-ai%2Fkimi-code%400.29.0/packages/agent-core/src/profile/context.ts"
	agentEvidenceURL       = "https://www.kimi.com/code/docs/en/kimi-code-cli/customization/agents.html"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Identity() agentconfig.ProviderIdentity {
	return agentconfig.ProviderIdentity{
		ID: providerID, Provider: "Moonshot AI", Surface: "Kimi Code CLI", ReportedVersion: "0.29.0 semantics checked 2026-07-24",
		AdapterID: "builtin/kimi/v1", Support: "preview", Depth: "repository-instructions-baseline",
	}
}

type resolver struct {
	view        *workspace.View
	target      string
	projectRoot string
	resolution  *agentconfig.Resolution
}

func (a *Adapter) Resolve(_ context.Context, view *workspace.View, target string, options provider.Options) (agentconfig.Resolution, error) {
	identity := a.Identity()
	normalizedTarget, err := view.NormalizeTarget(target)
	if err != nil {
		return agentconfig.Resolution{}, err
	}
	directories, err := view.DirectoriesToTarget(normalizedTarget)
	if err != nil {
		return agentconfig.Resolution{}, err
	}
	resolution := agentconfig.Resolution{
		Provider: identity, Target: normalizedTarget, ProjectRoot: view.ProjectRootDisplay(),
		Evidence: []agentconfig.EvidenceRecord{
			{
				ID: "kimi-code-agents-md-0.29.0", Kind: "official-source",
				Claim:    "Kimi Code CLI loads user instructions first, then project instruction files from the nearest Git root to the working directory.",
				Provider: providerID, Surface: "Kimi Code CLI", VersionRange: "0.29.0",
				SourceURL: instructionEvidenceURL, CheckedOn: "2026-07-24", Status: "verified",
			},
			{
				ID: "kimi-code-agent-profiles-0.29.0", Kind: "official-documentation",
				Claim:    "Custom main-agent profiles and SYSTEM.md can replace the default prompt that normally injects merged AGENTS.md content.",
				Provider: providerID, Surface: "Kimi Code CLI", VersionRange: "0.29.0",
				SourceURL: agentEvidenceURL, CheckedOn: "2026-07-24", Status: "documented",
			},
		},
	}

	for _, external := range options.ExternalSources {
		source := provider.ReadExternalPlainMarkdown(providerID, external, "opted-in Kimi user instruction loaded before project instructions", false)
		source.Content = strings.TrimSpace(source.Content)
		if source.Content == "" {
			continue
		}
		resolution.IncludedSources = append(resolution.IncludedSources, source)
	}

	rootIndex := len(directories) - 1
	for index := len(directories) - 1; index >= 0; index-- {
		marker := workspace.Join(directories[index], ".git")
		exists, existsErr := view.Exists(marker)
		if existsErr != nil {
			stub := repositoryStub(marker, "kimi-project-marker", "Kimi project marker could not be inspected")
			resolution.Findings = append(resolution.Findings, provider.ReadFailureFinding(providerID, normalizedTarget, stub, existsErr))
			continue
		}
		if exists {
			rootIndex = index
			break
		}
	}

	r := resolver{
		view: view, target: normalizedTarget, projectRoot: directories[rootIndex], resolution: &resolution,
	}
	for _, directory := range directories[rootIndex:] {
		r.include(workspace.Join(directory, ".kimi-code/AGENTS.md"), "kimi-brand-instruction", "Kimi-specific project instruction at this directory")
		for _, fileName := range []string{"AGENTS.md", "agents.md"} {
			if r.include(workspace.Join(directory, fileName), "kimi-generic-instruction", "first non-empty generic Kimi instruction candidate in this directory") {
				break
			}
		}
	}

	if renderedInstructionBytes(view, resolution.IncludedSources) > recommendedMaxBytes {
		resolution.Findings = append(resolution.Findings, agentconfig.Finding{
			Code: "ACI045", Severity: "warning", Title: "Kimi instruction guidance exceeds the recommended size",
			Summary:   "Kimi Code CLI keeps the full merged instruction content but warns when it exceeds the recommended 32 KiB budget.",
			Providers: []string{providerID}, Targets: []string{normalizedTarget}, Confidence: "high",
			Remediation: []string{"Trim redundant Kimi instruction files to reduce prompt cost and ambiguity."},
		})
	}
	resolution.Findings = append(resolution.Findings, agentconfig.Finding{
		Code: "ACI064", Severity: "info", Title: "Kimi main-agent prompt selection is not observed",
		Summary:   "The baseline predicts the default prompt's AGENTS.md injection; SYSTEM.md, --agent, and --agent-file selection were not executed.",
		Providers: []string{providerID}, Targets: []string{normalizedTarget}, Confidence: "high",
	})
	provider.FinalizeResolution(&resolution)
	return resolution, nil
}

func (r *resolver) include(logical, kind, reason string) bool {
	exists, err := r.view.Exists(logical)
	if err != nil {
		stub := repositoryStub(logical, kind, reason)
		r.resolution.Findings = append(r.resolution.Findings, provider.ReadFailureFinding(providerID, r.target, stub, err))
		return false
	}
	if !exists {
		return false
	}
	source, readErr := provider.ReadPlainMarkdown(r.view, providerID, logical, kind, reason, false)
	source.Scope.Base = r.projectRoot
	if readErr != nil {
		r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, source)
		r.resolution.Findings = append(r.resolution.Findings, provider.ReadFailureFinding(providerID, r.target, source, readErr))
		return false
	}
	source.Content = strings.TrimSpace(source.Content)
	if source.Content == "" {
		source.Scope.Status = "excluded"
		source.Scope.Reason = "Kimi skips empty instruction files"
		source.Reason = source.Scope.Reason
		r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, source)
		r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
			Code: "ACI003", Severity: "warning", Title: "Kimi instruction file is empty", Summary: source.Scope.Reason,
			Providers: []string{providerID}, Targets: []string{r.target}, Sources: []string{logical}, Confidence: "high",
		})
		return false
	}
	r.resolution.IncludedSources = append(r.resolution.IncludedSources, source)
	return true
}

func renderedInstructionBytes(view *workspace.View, sources []agentconfig.Source) int {
	total := 0
	for index, source := range sources {
		annotationPath := source.DisplayPath
		if source.Origin == "repository" && source.LogicalPath != "" {
			annotationPath = filepath.Join(view.Root(), filepath.FromSlash(source.LogicalPath))
		}
		if index > 0 {
			total += 2
		}
		total += len("<!-- From: ") + len(annotationPath) + len(" -->\n") + len(source.Content)
	}
	return total
}

func repositoryStub(logical, kind, reason string) agentconfig.Source {
	return agentconfig.Source{
		ID: providerID + ":" + kind + ":" + logical, Origin: "repository", LogicalPath: logical, DisplayPath: logical,
		Kind: kind, ContentVisibility: "metadata-only",
		Scope: agentconfig.Scope{Type: "hierarchical", Base: ".", Status: "unknown", Reason: reason}, Reason: reason,
	}
}

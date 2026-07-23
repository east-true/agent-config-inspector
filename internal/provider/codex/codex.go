package codex

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/east-true/agent-config-inspector/internal/provider"
	"github.com/east-true/agent-config-inspector/internal/workspace"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

const (
	defaultMaxBytes = 32 * 1024
	evidenceURL     = "https://developers.openai.com/codex/guides/agents-md"
	providerID      = "openai-codex/cli"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Identity() agentconfig.ProviderIdentity {
	return agentconfig.ProviderIdentity{
		ID: providerID, Provider: "OpenAI", Surface: "Codex CLI", ReportedVersion: "current as of 2026-07-24",
		AdapterID: "builtin/codex/v1", Support: "preview", Depth: "repository-instructions",
	}
}

type config struct {
	fallbacks []string
	maxBytes  int
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
		Evidence: []agentconfig.EvidenceRecord{{
			ID: "codex-agents-md-2026-07-24", Kind: "official-documentation",
			Claim:    "Codex loads one instruction file per directory from project root to target, with override and size-limit rules.",
			Provider: providerID, Surface: "Codex CLI", VersionRange: "current as of 2026-07-24",
			SourceURL: evidenceURL, CheckedOn: "2026-07-24", Status: "verified",
		}},
	}
	for _, external := range options.ExternalSources {
		resolution.IncludedSources = append(resolution.IncludedSources, provider.ReadExternalMarkdown(
			providerID, external, "opted-in Codex user instruction loaded before project instructions", false,
		))
	}

	settings := config{maxBytes: defaultMaxBytes}
	for _, dir := range directories {
		configPath := workspace.Join(dir, ".codex/config.toml")
		file, readErr := view.Read(configPath)
		if readErr != nil {
			exists, existsErr := view.Exists(configPath)
			if existsErr == nil && !exists {
				continue
			}
			stub := agentconfig.Source{ID: providerID + ":config:" + configPath, Origin: "repository", LogicalPath: configPath, DisplayPath: configPath, Kind: "codex-config", ContentVisibility: "metadata-only", SizeBytes: file.Size}
			resolution.Findings = append(resolution.Findings, provider.ReadFailureFinding(providerID, normalizedTarget, stub, readErr))
			continue
		}
		parsed, parseErr := parseConfig(string(file.Bytes), settings)
		if parseErr != nil {
			resolution.Findings = append(resolution.Findings, agentconfig.Finding{
				Code: "ACI023", Severity: "warning", Title: "Codex project configuration is only partially understood",
				Summary: fmt.Sprintf("%s: %v", configPath, parseErr), Providers: []string{providerID}, Targets: []string{normalizedTarget},
				Sources: []string{configPath}, Confidence: "high", Remediation: []string{"Check project_doc_fallback_filenames and project_doc_max_bytes syntax."},
			})
		}
		settings = parsed
	}

	combined := 0
	limitReached := false
	for _, dir := range directories {
		candidates := append([]string{"AGENTS.override.md", "AGENTS.md"}, settings.fallbacks...)
		selected := false
		for _, name := range candidates {
			logical := workspace.Join(dir, name)
			exists, existsErr := view.Exists(logical)
			if existsErr != nil {
				stub := agentconfig.Source{ID: providerID + ":instruction:" + logical, Origin: "repository", LogicalPath: logical, DisplayPath: logical, Kind: "codex-instruction", ContentVisibility: "metadata-only"}
				resolution.Findings = append(resolution.Findings, provider.ReadFailureFinding(providerID, normalizedTarget, stub, existsErr))
				continue
			}
			if !exists {
				continue
			}
			source, readErr := provider.ReadMarkdown(view, providerID, logical, "codex-instruction", "first non-empty Codex instruction candidate in this directory", false)
			if readErr != nil {
				resolution.ExcludedSources = append(resolution.ExcludedSources, source)
				resolution.Findings = append(resolution.Findings, provider.ReadFailureFinding(providerID, normalizedTarget, source, readErr))
				continue
			}
			if strings.TrimSpace(source.Content) == "" {
				source.Scope.Status = "excluded"
				source.Scope.Reason = "Codex skips empty instruction files"
				source.Reason = source.Scope.Reason
				resolution.ExcludedSources = append(resolution.ExcludedSources, source)
				continue
			}
			if selected {
				source.Scope.Status = "excluded"
				source.Scope.Reason = "a higher-priority instruction filename already won in this directory"
				source.Reason = source.Scope.Reason
				resolution.ExcludedSources = append(resolution.ExcludedSources, source)
				continue
			}
			selected = true
			if limitReached || combined+int(source.SizeBytes) > settings.maxBytes {
				limitReached = true
				source.Scope.Status = "excluded"
				source.Scope.Reason = fmt.Sprintf("combined Codex project instructions reached the %d-byte limit", settings.maxBytes)
				source.Reason = source.Scope.Reason
				resolution.ExcludedSources = append(resolution.ExcludedSources, source)
				resolution.Findings = append(resolution.Findings, agentconfig.Finding{
					Code: "ACI045", Severity: "warning", Title: "Codex instruction budget reached", Summary: source.Scope.Reason,
					Providers: []string{providerID}, Targets: []string{normalizedTarget}, Sources: []string{logical}, Confidence: "medium",
					Remediation: []string{"Reduce instruction size or raise project_doc_max_bytes in Codex configuration."},
				})
				continue
			}
			combined += int(source.SizeBytes)
			resolution.IncludedSources = append(resolution.IncludedSources, source)
		}
	}

	resolution.Findings = append(resolution.Findings, agentconfig.Finding{
		Code: "ACI021", Severity: "info", Title: "Codex project trust is a runtime condition",
		Summary:   "Static analysis assumes project-scoped Codex configuration is trusted; an untrusted runtime may ignore .codex layers.",
		Providers: []string{providerID}, Targets: []string{normalizedTarget}, Confidence: "high",
	})
	provider.FinalizeResolution(&resolution)
	return resolution, nil
}

func parseConfig(content string, base config) (config, error) {
	result := config{fallbacks: append([]string(nil), base.fallbacks...), maxBytes: base.maxBytes}
	var problems []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(strings.SplitN(scanner.Text(), "#", 2)[0])
		if line == "" {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "project_doc_max_bytes":
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || parsed <= 0 {
				problems = append(problems, "invalid project_doc_max_bytes")
			} else {
				result.maxBytes = parsed
			}
		case "project_doc_fallback_filenames":
			value = strings.TrimSpace(value)
			if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
				problems = append(problems, "fallback filenames must be a one-line string array")
				continue
			}
			var names []string
			for _, item := range strings.Split(strings.Trim(value, "[]"), ",") {
				name := strings.Trim(strings.TrimSpace(item), "\"'")
				if name == "" {
					continue
				}
				if path.Base(name) != name || name == "." || name == ".." {
					problems = append(problems, "fallback filenames must not contain directories")
					continue
				}
				names = append(names, name)
			}
			result.fallbacks = names
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		problems = append(problems, scanErr.Error())
	}
	if len(problems) > 0 {
		return result, errors.New(strings.Join(problems, "; "))
	}
	return result, nil
}

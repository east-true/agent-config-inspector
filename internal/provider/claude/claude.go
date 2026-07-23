package claude

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/east-true/agent-config-inspector/internal/parser"
	"github.com/east-true/agent-config-inspector/internal/provider"
	"github.com/east-true/agent-config-inspector/internal/workspace"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

const (
	evidenceURL = "https://code.claude.com/docs/en/memory"
	providerID  = "anthropic-claude-code/cli"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Identity() agentconfig.ProviderIdentity {
	return agentconfig.ProviderIdentity{
		ID: providerID, Provider: "Anthropic", Surface: "Claude Code", ReportedVersion: "current as of 2026-07-24",
		AdapterID: "builtin/claude/v1", Support: "preview", Depth: "repository-instructions",
	}
}

type resolver struct {
	view       *workspace.View
	target     string
	maxDepth   int
	seen       map[string]struct{}
	resolution *agentconfig.Resolution
}

func (a *Adapter) Resolve(_ context.Context, view *workspace.View, target string, opts provider.Options) (agentconfig.Resolution, error) {
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
			ID: "claude-memory-2026-07-24", Kind: "official-documentation",
			Claim:    "Claude Code loads hierarchical CLAUDE.md files, imports, and recursive path-scoped project rules.",
			Provider: providerID, Surface: "Claude Code", VersionRange: "current as of 2026-07-24",
			SourceURL: evidenceURL, CheckedOn: "2026-07-24", Status: "verified",
		}},
	}
	maxDepth := opts.MaxImportDepth
	if maxDepth <= 0 || maxDepth > 4 {
		maxDepth = 4
	}
	r := resolver{view: view, target: normalizedTarget, maxDepth: maxDepth, seen: make(map[string]struct{}), resolution: &resolution}
	for _, external := range opts.ExternalSources {
		source := provider.ReadExternalMarkdown(providerID, external, "opted-in Claude user instruction loaded before project instructions", true)
		if external.Kind == "claude-user-rule" {
			source.Scope.Type = "glob"
			matched, matchErr := matchesAny(source.Scope.Patterns, normalizedTarget)
			if matchErr != nil {
				source.Scope.Status = "unknown"
				source.Scope.Reason = "a redacted user rule contains an unsupported or malformed path glob"
				source.Reason = source.Scope.Reason
				resolution.ExcludedSources = append(resolution.ExcludedSources, source)
				resolution.Findings = append(resolution.Findings, invalidGlobFinding(normalizedTarget, source.DisplayPath))
				continue
			}
			if len(source.Scope.Patterns) > 0 && !matched {
				source.Scope.Status = "excluded"
				source.Scope.Reason = "target does not match the redacted user rule's paths globs"
				source.Reason = source.Scope.Reason
				resolution.ExcludedSources = append(resolution.ExcludedSources, source)
				continue
			}
		}
		resolution.IncludedSources = append(resolution.IncludedSources, source)
		if len(parser.ParseMarkdown(source.ID, []byte(source.Content), false).Imports) > 0 {
			resolution.Findings = append(resolution.Findings, agentconfig.Finding{
				Code: "ACI032", Severity: "info", Title: "Imports from redacted user instructions are not expanded",
				Summary:   "The preview inventories the opted-in user source but does not expose or traverse its external import graph.",
				Providers: []string{providerID}, Targets: []string{normalizedTarget}, Sources: []string{source.DisplayPath}, Confidence: "high",
			})
		}
	}

	// Project-root .claude/CLAUDE.md is the documented alternative project location.
	r.includeWithImports(".claude/CLAUDE.md", "claude-memory", "project instructions from .claude/CLAUDE.md", nil, 0)

	r.includeRules()

	for _, dir := range directories {
		r.includeWithImports(workspace.Join(dir, "CLAUDE.md"), "claude-memory", "hierarchical CLAUDE.md from project root to target", nil, 0)
		r.includeWithImports(workspace.Join(dir, "CLAUDE.local.md"), "claude-local-memory", "local hierarchical memory loaded after CLAUDE.md in the same directory", nil, 0)
	}

	provider.FinalizeResolution(&resolution)
	return resolution, nil
}

func (r *resolver) includeRules() {
	files, err := r.view.WalkFiles(".claude/rules", markdownFile)
	if err != nil {
		stub := agentconfig.Source{ID: "claude:rules:.claude/rules", Origin: "repository", LogicalPath: ".claude/rules", DisplayPath: ".claude/rules", Kind: "claude-rule", ContentVisibility: "metadata-only"}
		r.resolution.Findings = append(r.resolution.Findings, provider.ReadFailureFinding(providerID, r.target, stub, err))
		return
	}
	for _, logical := range files {
		if _, ok := r.seen[logical]; ok {
			continue
		}
		r.seen[logical] = struct{}{}
		source, readErr := provider.ReadMarkdown(r.view, providerID, logical, "claude-rule", "recursive project rule under .claude/rules", true)
		if readErr != nil {
			r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, source)
			r.resolution.Findings = append(r.resolution.Findings, provider.ReadFailureFinding(providerID, r.target, source, readErr))
			continue
		}
		source.Scope.Type = "glob"
		source.Scope.Base = "."
		if len(source.Scope.Patterns) == 0 {
			source.Scope.Status = "applies"
			source.Scope.Reason = "rule has no paths frontmatter and loads unconditionally"
		} else if matched, matchErr := matchesAny(source.Scope.Patterns, r.target); matchErr != nil {
			source.Scope.Status = "unknown"
			source.Scope.Reason = "rule contains an unsupported or malformed paths glob"
			source.Reason = source.Scope.Reason
			r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, source)
			r.resolution.Findings = append(r.resolution.Findings, invalidGlobFinding(r.target, source.DisplayPath))
			continue
		} else if matched {
			source.Scope.Status = "applies"
			source.Scope.Reason = "target matches at least one paths glob"
		} else {
			source.Scope.Status = "excluded"
			source.Scope.Reason = "target does not match the rule's paths globs"
			source.Reason = source.Scope.Reason
			r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, source)
			continue
		}
		source.Reason = source.Scope.Reason
		r.resolution.IncludedSources = append(r.resolution.IncludedSources, source)
		r.includeImports(source, nil, 0)
	}
}

func markdownFile(logical string, _ fs.DirEntry) bool {
	return strings.EqualFold(path.Ext(logical), ".md")
}

func (r *resolver) includeWithImports(logical, kind, reason string, stack []string, depth int) {
	if _, ok := r.seen[logical]; ok {
		return
	}
	exists, err := r.view.Exists(logical)
	if err != nil {
		stub := agentconfig.Source{ID: "claude:" + kind + ":" + logical, Origin: "repository", LogicalPath: logical, DisplayPath: logical, Kind: kind, ContentVisibility: "metadata-only"}
		r.resolution.Findings = append(r.resolution.Findings, provider.ReadFailureFinding(providerID, r.target, stub, err))
		return
	}
	if !exists {
		return
	}
	r.seen[logical] = struct{}{}
	source, readErr := provider.ReadMarkdown(r.view, providerID, logical, kind, reason, true)
	if readErr != nil {
		r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, source)
		r.resolution.Findings = append(r.resolution.Findings, provider.ReadFailureFinding(providerID, r.target, source, readErr))
		return
	}
	r.resolution.IncludedSources = append(r.resolution.IncludedSources, source)
	r.includeImports(source, stack, depth)
}

func (r *resolver) includeImports(source agentconfig.Source, stack []string, depth int) {
	parsed := parser.ParseMarkdown(source.ID, []byte(source.Content), false)
	if len(parsed.Imports) == 0 {
		return
	}
	for _, imported := range parsed.Imports {
		if depth >= r.maxDepth {
			r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
				Code: "ACI024", Severity: "warning", Title: "Claude import depth limit reached",
				Summary:   fmt.Sprintf("An import from %s was not expanded beyond %d hops.", source.DisplayPath, r.maxDepth),
				Providers: []string{providerID}, Targets: []string{r.target}, Sources: []string{source.DisplayPath}, Confidence: "high",
			})
			continue
		}
		if strings.HasPrefix(imported, "~") || path.IsAbs(strings.ReplaceAll(imported, "\\", "/")) {
			r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, externalImportStub(source))
			r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
				Code: "ACI005", Severity: "warning", Title: "External Claude import was not scanned",
				Summary:   "A Claude memory file imports outside the selected workspace. The external path is redacted and excluded by default.",
				Providers: []string{providerID}, Targets: []string{r.target}, Sources: []string{source.DisplayPath, "<external-import>"}, Confidence: "high",
				Remediation: []string{"Review external imports locally; do not publish user-context output without redaction."},
			})
			continue
		}
		logical := parser.ResolveRelative(source.LogicalPath, imported)
		if logical == ".." || strings.HasPrefix(logical, "../") {
			r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, externalImportStub(source))
			r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
				Code: "ACI005", Severity: "warning", Title: "Claude import escapes the workspace",
				Summary:   "A relative Claude import resolves outside the selected workspace and was excluded.",
				Providers: []string{providerID}, Targets: []string{r.target}, Sources: []string{source.DisplayPath, "<external-import>"}, Confidence: "high",
			})
			continue
		}
		if contains(stack, logical) || logical == source.LogicalPath {
			r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
				Code: "ACI025", Severity: "warning", Title: "Claude import cycle detected",
				Summary: fmt.Sprintf("Import expansion stopped at %s.", logical), Providers: []string{providerID},
				Targets: []string{r.target}, Sources: []string{source.DisplayPath, logical}, Confidence: "high",
			})
			continue
		}
		exists, existsErr := r.view.Exists(logical)
		if existsErr != nil {
			stub := missingImportStub(logical)
			r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, stub)
			r.resolution.Findings = append(r.resolution.Findings, provider.ReadFailureFinding(providerID, r.target, stub, existsErr))
			continue
		}
		if !exists {
			stub := missingImportStub(logical)
			r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, stub)
			r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
				Code: "ACI026", Severity: "warning", Title: "Claude import target is missing",
				Summary: "An in-workspace Claude import refers to a file that does not exist.", Providers: []string{providerID},
				Targets: []string{r.target}, Sources: []string{source.DisplayPath, logical}, Confidence: "high",
				Remediation: []string{"Create the imported file or remove the stale import."},
			})
			continue
		}
		nextStack := append(append([]string(nil), stack...), source.LogicalPath)
		r.includeWithImports(logical, "claude-import", "file imported by Claude memory", nextStack, depth+1)
	}
}

func matchesAny(patterns []string, target string) (bool, error) {
	target = strings.TrimPrefix(target, "./")
	for _, pattern := range patterns {
		matched, err := parser.MatchGlob(pattern, target)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func externalImportStub(source agentconfig.Source) agentconfig.Source {
	return agentconfig.Source{
		ID:     "claude:external-import:" + source.ID,
		Origin: "user", DisplayPath: "<external-import>", Kind: "claude-import", ContentVisibility: "hidden",
		Scope:  agentconfig.Scope{Type: "import", Status: "excluded", Reason: "outside workspace privacy boundary"},
		Reason: "external path redacted and not read",
	}
}

func missingImportStub(logical string) agentconfig.Source {
	return agentconfig.Source{
		ID: providerID + ":missing-import:" + logical, Origin: "repository", LogicalPath: logical, DisplayPath: logical,
		Kind: "claude-import", ContentVisibility: "metadata-only",
		Scope:  agentconfig.Scope{Type: "import", Status: "excluded", Reason: "import target is missing"},
		Reason: "import target is missing",
	}
}

func invalidGlobFinding(target, displayPath string) agentconfig.Finding {
	return agentconfig.Finding{
		Code: "ACI027", Severity: "warning", Title: "Claude path glob could not be evaluated",
		Summary:   "A Claude rule contains a malformed or unsupported paths glob and was not predicted to apply.",
		Providers: []string{providerID}, Targets: []string{target}, Sources: []string{displayPath}, Confidence: "high",
		Remediation: []string{"Use a documented Claude Code paths glob supported by this adapter."},
	}
}

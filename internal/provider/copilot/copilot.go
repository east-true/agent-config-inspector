package copilot

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
	providerID            = "github-copilot/cli"
	customInstructionsURL = "https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-custom-instructions"
	commandReferenceURL   = "https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference"
	surfaceSupportURL     = "https://docs.github.com/en/copilot/reference/custom-instructions-support"
	releaseURL            = "https://github.com/github/copilot-cli/releases/tag/v1.0.73"
	checkedCopilotVersion = "v1.0.73"
	defaultMaxImportDepth = 5
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Identity() agentconfig.ProviderIdentity {
	return agentconfig.ProviderIdentity{
		ID: providerID, Provider: "GitHub", Surface: "Copilot CLI", ReportedVersion: checkedCopilotVersion + " semantics checked 2026-07-24",
		AdapterID: "builtin/copilot/v1", Support: "preview", Depth: "repository-custom-instructions-baseline",
	}
}

type resolver struct {
	view           *workspace.View
	target         string
	projectRoot    string
	maxDepth       int
	seenPaths      map[string]struct{}
	primaryDigests map[string]string
	resolution     *agentconfig.Resolution
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
	rootIndex, markerFailures := nearestRepositoryRoot(view, directories)
	projectDirectories := directories[rootIndex:]
	resolution := agentconfig.Resolution{
		Provider: identity, Target: normalizedTarget, ProjectRoot: view.ProjectRootDisplay(),
		Evidence: []agentconfig.EvidenceRecord{
			{
				ID: "copilot-cli-custom-instructions-v1.0.73", Kind: "official-documentation",
				Claim:    "Copilot CLI combines repository, agent, and path-specific custom instructions from documented standard locations and expands supported relative imports.",
				Provider: providerID, Surface: "Copilot CLI", VersionRange: checkedCopilotVersion,
				SourceURL: customInstructionsURL, CheckedOn: "2026-07-24", Status: "documented",
			},
			{
				ID: "copilot-cli-import-reference-v1.0.73", Kind: "official-documentation",
				Claim:    "Supported instruction families expand line-prefixed @path imports relative to the instruction file with recursive depth, cycle, and size guards.",
				Provider: providerID, Surface: "Copilot CLI", VersionRange: checkedCopilotVersion,
				SourceURL: commandReferenceURL, CheckedOn: "2026-07-24", Status: "documented",
			},
			{
				ID: "copilot-custom-instruction-surface-matrix-2026-07-24", Kind: "official-documentation",
				Claim:    "Custom-instruction capabilities differ across Copilot CLI, coding agent, code review, and IDE surfaces.",
				Provider: providerID, Surface: "Copilot CLI", VersionRange: "current as of 2026-07-24",
				SourceURL: surfaceSupportURL, CheckedOn: "2026-07-24", Status: "documented",
			},
			{
				ID: "copilot-cli-stable-release-v1.0.73", Kind: "official-release",
				Claim:    "v1.0.73 was the latest stable Copilot CLI release when this adapter baseline was checked.",
				Provider: providerID, Surface: "Copilot CLI", VersionRange: checkedCopilotVersion,
				SourceURL: releaseURL, CheckedOn: "2026-07-24", Status: "verified",
			},
		},
	}
	maxDepth := options.MaxImportDepth
	if maxDepth <= 0 || maxDepth > defaultMaxImportDepth {
		maxDepth = defaultMaxImportDepth
	}
	r := resolver{
		view: view, target: normalizedTarget, projectRoot: projectDirectories[0], maxDepth: maxDepth,
		seenPaths: make(map[string]struct{}), primaryDigests: make(map[string]string), resolution: &resolution,
	}
	for _, failure := range markerFailures {
		stub := repositoryStub(failure.logical, "copilot-project-marker", "Copilot repository marker could not be inspected")
		resolution.Findings = append(resolution.Findings, provider.ReadFailureFinding(providerID, normalizedTarget, stub, failure.err))
	}

	for _, external := range options.ExternalSources {
		r.includeExternal(external)
	}

	for _, directory := range projectDirectories {
		r.includePrimary(workspace.Join(directory, ".github/copilot-instructions.md"), "copilot-repository-instruction", "repository-wide Copilot instruction in a documented standard location", true)
		r.includePrimary(workspace.Join(directory, "AGENTS.md"), "copilot-agent-instruction", "AGENTS.md in a documented Copilot CLI standard location", true)
		r.includePrimary(workspace.Join(directory, "CLAUDE.md"), "copilot-compatible-instruction", "CLAUDE.md in a documented Copilot CLI standard location", true)
		r.includePrimary(workspace.Join(directory, ".claude/CLAUDE.md"), "copilot-compatible-instruction", ".claude/CLAUDE.md in a documented Copilot CLI standard location", true)
		r.includePrimary(workspace.Join(directory, "GEMINI.md"), "copilot-compatible-instruction", "GEMINI.md in a documented Copilot CLI standard location", false)
	}

	// The selected project root is the scanner's initial working directory, so
	// each later directory is a documented location nested in the target path.
	// A runtime cwd below that root would make some earlier directories
	// "intermediate" and is outside this static request model.
	for _, base := range projectDirectories {
		r.includeModularDirectory(base)
	}

	resolution.Findings = append(resolution.Findings, agentconfig.Finding{
		Code: "ACI064", Severity: "info", Title: "Copilot CLI runtime instruction state is predicted",
		Summary:   "Applicable files are shown in a deterministic inventory order, not as a precedence claim; session-disabled instructions and files discovered after other runtime file access are not observed.",
		Providers: []string{providerID}, Targets: []string{normalizedTarget}, Confidence: "high",
	})
	provider.FinalizeResolution(&resolution)
	return resolution, nil
}

type markerFailure struct {
	logical string
	err     error
}

func nearestRepositoryRoot(view *workspace.View, directories []string) (int, []markerFailure) {
	var failures []markerFailure
	for index := len(directories) - 1; index >= 0; index-- {
		logical := workspace.Join(directories[index], ".git")
		exists, err := view.Exists(logical)
		if err != nil {
			failures = append(failures, markerFailure{logical: logical, err: err})
			continue
		}
		if exists {
			return index, failures
		}
	}
	return 0, failures
}

func (r *resolver) includeExternal(external provider.ExternalSource) {
	modular := external.Kind == "copilot-user-modular-instruction"
	var source agentconfig.Source
	if modular {
		source = provider.ReadExternalMarkdown(providerID, external, "opted-in path-specific Copilot user instruction", false)
		source.Scope.Type = "glob"
		patterns, found, err := parser.ParseFrontmatterCSV(external.Content, "applyTo")
		if err != nil || !found {
			r.excludeInvalidGlob(&source, "a redacted user instruction has missing or malformed applyTo frontmatter")
			return
		}
		source.Scope.Patterns = patterns
		if strings.TrimSpace(source.Content) == "" {
			r.excludeEmpty(&source)
			return
		}
		matched, matchErr := matchesAny(patterns, r.target, ".")
		if matchErr != nil {
			r.excludeInvalidGlob(&source, "a redacted user instruction contains an unsupported or malformed applyTo glob")
			return
		}
		if !matched {
			source.Scope.Status = "excluded"
			source.Scope.Reason = "target does not match the redacted user instruction's applyTo globs"
			source.Reason = source.Scope.Reason
			r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, source)
			return
		}
		source.Scope.Reason = "target matches at least one redacted user instruction applyTo glob"
		source.Reason = source.Scope.Reason
	} else {
		source = provider.ReadExternalPlainMarkdown(providerID, external, "opted-in Copilot user instruction", false)
	}
	if modular {
		r.resolution.IncludedSources = append(r.resolution.IncludedSources, source)
		return
	}
	if !r.includeResolvedPrimary(source, parser.RawContentDigest(external.Content).Value) {
		return
	}
	if len(copilotImports(source.Content)) > 0 {
		r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
			Code: "ACI032", Severity: "info", Title: "Imports from redacted Copilot user instructions are not expanded",
			Summary:   "The opted-in user source is inventoried without exposing or traversing its private import graph.",
			Providers: []string{providerID}, Targets: []string{r.target}, Sources: []string{source.DisplayPath}, Confidence: "high",
		})
	}
}

func (r *resolver) includePrimary(logical, kind, reason string, expandImports bool) {
	if _, seen := r.seenPaths[logical]; seen {
		return
	}
	exists, err := r.view.Exists(logical)
	if err != nil {
		stub := repositoryStub(logical, kind, reason)
		r.resolution.Findings = append(r.resolution.Findings, provider.ReadFailureFinding(providerID, r.target, stub, err))
		return
	}
	if !exists {
		return
	}
	r.seenPaths[logical] = struct{}{}
	source, readErr := provider.ReadPlainMarkdown(r.view, providerID, logical, kind, reason, false)
	source.Scope.Base = r.projectRoot
	if readErr != nil {
		r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, source)
		r.resolution.Findings = append(r.resolution.Findings, provider.ReadFailureFinding(providerID, r.target, source, readErr))
		return
	}
	digest := ""
	if source.RawDigest != nil {
		digest = source.RawDigest.Value
	}
	if !r.includeResolvedPrimary(source, digest) {
		return
	}
	if expandImports {
		r.includeImports(source, nil, 0)
	}
}

func (r *resolver) includeResolvedPrimary(source agentconfig.Source, digest string) bool {
	if strings.TrimSpace(source.Content) == "" {
		r.excludeEmpty(&source)
		return false
	}
	if prior, duplicate := r.primaryDigests[digest]; digest != "" && duplicate {
		source.Scope.Status = "excluded"
		source.Scope.Reason = "Copilot CLI removes an identical general instruction already discovered"
		source.Reason = source.Scope.Reason
		r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, source)
		r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
			Code: "ACI041", Severity: "info", Title: "Duplicate Copilot instruction removed",
			Summary:   "Copilot CLI removes identical copies of general user, repository-wide, and agent instructions.",
			Providers: []string{providerID}, Targets: []string{r.target}, Sources: []string{prior, source.DisplayPath}, Confidence: "high",
		})
		return false
	}
	if digest != "" {
		r.primaryDigests[digest] = source.DisplayPath
	}
	r.resolution.IncludedSources = append(r.resolution.IncludedSources, source)
	return true
}

func (r *resolver) includeModularDirectory(base string) {
	directory := workspace.Join(base, ".github/instructions")
	files, err := r.view.WalkFiles(directory, func(logical string, entry fs.DirEntry) bool {
		return !entry.IsDir() && strings.HasSuffix(logical, ".instructions.md")
	})
	if err != nil {
		stub := repositoryStub(directory, "copilot-modular-instruction", "path-specific Copilot instruction directory")
		r.resolution.Findings = append(r.resolution.Findings, provider.ReadFailureFinding(providerID, r.target, stub, err))
		return
	}
	for _, logical := range files {
		r.includeModular(logical, base)
	}
}

func (r *resolver) includeModular(logical, base string) {
	if _, seen := r.seenPaths[logical]; seen {
		return
	}
	r.seenPaths[logical] = struct{}{}
	file, err := r.view.Read(logical)
	if err != nil {
		stub := repositoryStub(logical, "copilot-modular-instruction", "path-specific Copilot instruction")
		r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, stub)
		r.resolution.Findings = append(r.resolution.Findings, provider.ReadFailureFinding(providerID, r.target, stub, err))
		return
	}
	source, err := provider.ReadMarkdown(r.view, providerID, logical, "copilot-modular-instruction", "path-specific Copilot instruction", false)
	if err != nil {
		r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, source)
		r.resolution.Findings = append(r.resolution.Findings, provider.ReadFailureFinding(providerID, r.target, source, err))
		return
	}
	source.Scope.Type = "glob"
	source.Scope.Base = base
	patterns, found, parseErr := parser.ParseFrontmatterCSV(file.Bytes, "applyTo")
	if parseErr != nil || !found {
		r.excludeInvalidGlob(&source, "the modular instruction has missing or malformed applyTo frontmatter")
		return
	}
	source.Scope.Patterns = patterns
	if strings.TrimSpace(source.Content) == "" {
		r.excludeEmpty(&source)
		return
	}
	matched, matchErr := matchesAny(patterns, r.target, base)
	if matchErr != nil {
		r.excludeInvalidGlob(&source, "the modular instruction contains an unsupported or malformed applyTo glob")
		return
	}
	if !matched {
		source.Scope.Status = "excluded"
		source.Scope.Reason = "target does not match the instruction's applyTo globs"
		source.Reason = source.Scope.Reason
		r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, source)
		return
	}
	source.Scope.Status = "applies"
	source.Scope.Reason = "target matches at least one applyTo glob"
	source.Reason = source.Scope.Reason
	r.resolution.IncludedSources = append(r.resolution.IncludedSources, source)
}

func (r *resolver) excludeInvalidGlob(source *agentconfig.Source, reason string) {
	source.Scope.Status = "unknown"
	source.Scope.Reason = reason
	source.Reason = reason
	r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, *source)
	r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
		Code: "ACI027", Severity: "warning", Title: "Copilot applyTo glob could not be evaluated",
		Summary:   "A path-specific Copilot instruction was not predicted to apply because its applyTo metadata is absent, malformed, or unsupported.",
		Providers: []string{providerID}, Targets: []string{r.target}, Sources: []string{source.DisplayPath}, Confidence: "high",
		Remediation: []string{"Use a documented comma-separated Copilot applyTo glob in YAML frontmatter."},
	})
}

func (r *resolver) includeImports(source agentconfig.Source, stack []string, depth int) {
	for _, imported := range copilotImports(source.Content) {
		if depth >= r.maxDepth {
			r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
				Code: "ACI024", Severity: "warning", Title: "Copilot import depth limit reached",
				Summary:   fmt.Sprintf("An import from %s was not expanded beyond %d hops.", source.DisplayPath, r.maxDepth),
				Providers: []string{providerID}, Targets: []string{r.target}, Sources: []string{source.DisplayPath}, Confidence: "high",
			})
			continue
		}
		if unsafeImport(imported) {
			r.excludeExternalImport(source, "A Copilot instruction import uses an absolute, home-relative, or external path and was excluded.")
			continue
		}
		logical := parser.ResolveRelative(source.LogicalPath, imported)
		if !withinProject(logical, r.projectRoot) {
			r.excludeExternalImport(source, "A Copilot instruction import resolves outside the selected repository and was excluded.")
			continue
		}
		if logical == source.LogicalPath || contains(stack, logical) {
			r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
				Code: "ACI025", Severity: "warning", Title: "Copilot import cycle detected",
				Summary:   "Import expansion stopped after encountering a previously active repository source.",
				Providers: []string{providerID}, Targets: []string{r.target}, Sources: []string{source.DisplayPath, logical}, Confidence: "high",
			})
			continue
		}
		if _, seen := r.seenPaths[logical]; seen {
			continue
		}
		exists, existsErr := r.view.Exists(logical)
		if existsErr != nil {
			stub := repositoryStub(logical, "copilot-import", "Copilot import target could not be inspected")
			r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, stub)
			r.resolution.Findings = append(r.resolution.Findings, provider.ReadFailureFinding(providerID, r.target, stub, existsErr))
			continue
		}
		if !exists {
			stub := repositoryStub(logical, "copilot-import", "Copilot import target is missing")
			stub.Scope.Status = "excluded"
			r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, stub)
			r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
				Code: "ACI026", Severity: "warning", Title: "Copilot import target is missing",
				Summary:   "An in-repository Copilot instruction import refers to a file that does not exist.",
				Providers: []string{providerID}, Targets: []string{r.target}, Sources: []string{source.DisplayPath, logical}, Confidence: "high",
				Remediation: []string{"Create the imported file or remove the stale import."},
			})
			continue
		}
		r.seenPaths[logical] = struct{}{}
		importedSource, readErr := provider.ReadPlainMarkdown(r.view, providerID, logical, "copilot-import", "file imported by a Copilot instruction", false)
		importedSource.Scope.Type = "import"
		importedSource.Scope.Base = r.projectRoot
		if readErr != nil {
			r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, importedSource)
			r.resolution.Findings = append(r.resolution.Findings, provider.ReadFailureFinding(providerID, r.target, importedSource, readErr))
			continue
		}
		if strings.TrimSpace(importedSource.Content) == "" {
			r.excludeEmpty(&importedSource)
			continue
		}
		r.resolution.IncludedSources = append(r.resolution.IncludedSources, importedSource)
		nextStack := append(append([]string(nil), stack...), source.LogicalPath)
		r.includeImports(importedSource, nextStack, depth+1)
	}
}

func (r *resolver) excludeEmpty(source *agentconfig.Source) {
	source.Scope.Status = "excluded"
	source.Scope.Reason = "the source contains no effective Copilot instruction text"
	source.Reason = source.Scope.Reason
	r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, *source)
	r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
		Code: "ACI003", Severity: "warning", Title: "Copilot instruction file is empty", Summary: source.Scope.Reason,
		Providers: []string{providerID}, Targets: []string{r.target}, Sources: []string{source.DisplayPath}, Confidence: "high",
	})
}

func copilotImports(content string) []string {
	var imports []string
	seen := make(map[string]struct{})
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || !strings.HasPrefix(trimmed, "@") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(trimmed, "@"))
		if len(fields) == 0 {
			continue
		}
		candidate := strings.TrimRight(fields[0], ".,;:!?)\"]}")
		if !strings.Contains(candidate, "/") && !strings.Contains(candidate, ".") && candidate != "README" {
			continue
		}
		if _, duplicate := seen[candidate]; duplicate {
			continue
		}
		seen[candidate] = struct{}{}
		imports = append(imports, candidate)
	}
	return imports
}

func (r *resolver) excludeExternalImport(source agentconfig.Source, summary string) {
	stub := agentconfig.Source{
		ID: providerID + ":copilot-import:" + source.ID, Origin: "user", DisplayPath: "<external-import>",
		Kind: "copilot-import", ContentVisibility: "hidden",
		Scope:  agentconfig.Scope{Type: "import", Status: "excluded", Reason: "outside repository privacy boundary"},
		Reason: "external path redacted and not read",
	}
	r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, stub)
	r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
		Code: "ACI005", Severity: "warning", Title: "External Copilot import was not scanned", Summary: summary,
		Providers: []string{providerID}, Targets: []string{r.target}, Sources: []string{source.DisplayPath, "<external-import>"}, Confidence: "high",
		Remediation: []string{"Keep Copilot CLI imports relative and inside the repository."},
	})
}

func matchesAny(patterns []string, target, base string) (bool, error) {
	relative, ok := relativeToBase(target, base)
	if !ok {
		return false, nil
	}
	for _, pattern := range patterns {
		matched, err := parser.MatchGlob(pattern, relative)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

func relativeToBase(target, base string) (string, bool) {
	if base == "." || base == "" {
		return strings.TrimPrefix(target, "./"), true
	}
	if target == base {
		return ".", true
	}
	prefix := strings.TrimSuffix(base, "/") + "/"
	if !strings.HasPrefix(target, prefix) {
		return "", false
	}
	return strings.TrimPrefix(target, prefix), true
}

func unsafeImport(imported string) bool {
	value := strings.TrimSpace(strings.ReplaceAll(imported, "\\", "/"))
	return value == "" || strings.HasPrefix(value, "~") || path.IsAbs(value) ||
		(len(value) >= 2 && value[1] == ':') || strings.Contains(value, "://")
}

func withinProject(logical, root string) bool {
	if root == "." {
		return logical != ".." && !strings.HasPrefix(logical, "../")
	}
	return logical == root || strings.HasPrefix(logical, strings.TrimSuffix(root, "/")+"/")
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func repositoryStub(logical, kind, reason string) agentconfig.Source {
	return agentconfig.Source{
		ID: providerID + ":" + kind + ":" + logical, Origin: "repository", LogicalPath: logical, DisplayPath: logical,
		Kind: kind, ContentVisibility: "metadata-only",
		Scope: agentconfig.Scope{Type: "hierarchical", Base: ".", Status: "unknown", Reason: reason}, Reason: reason,
	}
}

package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/east-true/agent-config-inspector/internal/parser"
	"github.com/east-true/agent-config-inspector/internal/provider"
	"github.com/east-true/agent-config-inspector/internal/workspace"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

const (
	contextEvidenceURL = "https://geminicli.com/docs/cli/gemini-md/"
	importEvidenceURL  = "https://geminicli.com/docs/reference/memport/"
	providerID         = "google-gemini/cli"
	defaultContextFile = "GEMINI.md"
	defaultImportDepth = 5
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Identity() agentconfig.ProviderIdentity {
	return agentconfig.ProviderIdentity{
		ID: providerID, Provider: "Google", Surface: "Gemini CLI", ReportedVersion: "v0.50.0 semantics checked 2026-07-24",
		AdapterID: "builtin/gemini/v1", Support: "preview", Depth: "repository-context-files",
	}
}

type settings struct {
	fileNames       []string
	boundaryMarkers []string
}

type resolver struct {
	view        *workspace.View
	target      string
	projectRoot string
	maxDepth    int
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
				ID: "gemini-context-files-v0.50.0", Kind: "official-documentation-and-source",
				Claim:    "Gemini CLI loads configured context files hierarchically and discovers target-specific context just in time.",
				Provider: providerID, Surface: "Gemini CLI", VersionRange: "v0.50.0",
				SourceURL: contextEvidenceURL, CheckedOn: "2026-07-24", Status: "documented",
			},
			{
				ID: "gemini-memory-imports-v0.50.0", Kind: "official-documentation-and-source",
				Claim:    "Gemini CLI expands bounded, in-project @path imports while rejecting cycles and unsafe paths.",
				Provider: providerID, Surface: "Gemini CLI", VersionRange: "v0.50.0",
				SourceURL: importEvidenceURL, CheckedOn: "2026-07-24", Status: "documented",
			},
		},
	}

	for _, external := range options.ExternalSources {
		source := provider.ReadExternalMarkdown(providerID, external, "opted-in Gemini global context loaded before project context", false)
		resolution.IncludedSources = append(resolution.IncludedSources, source)
		if len(geminiImports(parser.ParseMarkdown(source.ID, []byte(source.Content), false).Content)) > 0 {
			resolution.Findings = append(resolution.Findings, agentconfig.Finding{
				Code: "ACI032", Severity: "info", Title: "Imports from redacted Gemini user context are not expanded",
				Summary:   "The user context is inventoried without exposing or traversing its private import graph.",
				Providers: []string{providerID}, Targets: []string{normalizedTarget}, Sources: []string{source.DisplayPath}, Confidence: "high",
			})
		}
	}

	initialRootIndex := findBoundaryIndex(view, directories, []string{".git"})
	configured := defaultSettings()
	configPath := workspace.Join(directories[initialRootIndex], ".gemini/settings.json")
	if parsed, found, parseErr := readSettings(view, configPath); parseErr != nil {
		resolution.Findings = append(resolution.Findings, agentconfig.Finding{
			Code: "ACI023", Severity: "warning", Title: "Gemini project settings are only partially understood",
			Summary:   "The project context settings are malformed or contain an unsafe context path; documented defaults were used where necessary.",
			Providers: []string{providerID}, Targets: []string{normalizedTarget}, Sources: []string{configPath}, Confidence: "high",
			Remediation: []string{"Validate context.fileName and context.memoryBoundaryMarkers in .gemini/settings.json."},
		})
		configured = parsed
	} else if found {
		configured = parsed
	}

	rootIndex := findBoundaryIndex(view, directories, configured.boundaryMarkers)
	maxDepth := options.MaxImportDepth
	if maxDepth <= 0 || maxDepth > defaultImportDepth {
		maxDepth = defaultImportDepth
	}
	r := resolver{
		view: view, target: normalizedTarget, projectRoot: directories[rootIndex], maxDepth: maxDepth, resolution: &resolution,
	}
	for _, directory := range directories[rootIndex:] {
		for _, fileName := range configured.fileNames {
			r.includeContext(workspace.Join(directory, fileName), nil, 0)
		}
	}

	resolution.Findings = append(resolution.Findings, agentconfig.Finding{
		Code: "ACI064", Severity: "info", Title: "Gemini JIT context activation is predicted",
		Summary:   "The selected target is treated as an accessed path; runtime tool access and the installed Gemini CLI were not executed.",
		Providers: []string{providerID}, Targets: []string{normalizedTarget}, Confidence: "high",
	})
	provider.FinalizeResolution(&resolution)
	return resolution, nil
}

func defaultSettings() settings {
	return settings{fileNames: []string{defaultContextFile}, boundaryMarkers: []string{".git"}}
}

func readSettings(view *workspace.View, logical string) (settings, bool, error) {
	defaults := defaultSettings()
	exists, err := view.Exists(logical)
	if err != nil {
		return defaults, false, err
	}
	if !exists {
		return defaults, false, nil
	}
	file, err := view.Read(logical)
	if err != nil {
		return defaults, true, err
	}
	var raw struct {
		Context struct {
			FileName              json.RawMessage `json:"fileName"`
			MemoryBoundaryMarkers json.RawMessage `json:"memoryBoundaryMarkers"`
		} `json:"context"`
	}
	if err := json.Unmarshal(file.Bytes, &raw); err != nil {
		return defaults, true, err
	}
	result := defaults
	var problems []string
	if len(raw.Context.FileName) > 0 {
		var names []string
		var single string
		if err := json.Unmarshal(raw.Context.FileName, &single); err == nil {
			names = []string{single}
		} else if err := json.Unmarshal(raw.Context.FileName, &names); err != nil {
			problems = append(problems, "context.fileName must be a string or string array")
		}
		var safeNames []string
		for _, name := range names {
			if normalized, ok := safeRelativeName(name); ok {
				safeNames = appendUnique(safeNames, normalized)
			} else {
				problems = append(problems, "context.fileName contains an unsafe path")
			}
		}
		if len(safeNames) > 0 {
			result.fileNames = appendUnique(safeNames, defaultContextFile)
		}
	}
	if len(raw.Context.MemoryBoundaryMarkers) > 0 {
		var markers []string
		if err := json.Unmarshal(raw.Context.MemoryBoundaryMarkers, &markers); err != nil {
			problems = append(problems, "context.memoryBoundaryMarkers must be a string array")
		} else {
			var safeMarkers []string
			for _, marker := range markers {
				if normalized, ok := safeBoundaryMarker(marker); ok {
					safeMarkers = appendUnique(safeMarkers, normalized)
				} else {
					problems = append(problems, "context.memoryBoundaryMarkers contains an unsafe marker")
				}
			}
			if len(markers) == 0 || len(safeMarkers) > 0 {
				result.boundaryMarkers = safeMarkers
			}
		}
	}
	if len(problems) > 0 {
		return result, true, errors.New(strings.Join(problems, "; "))
	}
	return result, true, nil
}

func safeRelativeName(value string) (string, bool) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || path.IsAbs(value) || hasWindowsDrivePrefix(value) {
		return "", false
	}
	for _, character := range value {
		if character == 0 || unicode.IsControl(character) {
			return "", false
		}
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return cleaned, true
}

func safeBoundaryMarker(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "..") || strings.ContainsAny(value, "/\\") || path.Base(value) != value || value == "." {
		return "", false
	}
	for _, character := range value {
		if character == 0 || unicode.IsControl(character) {
			return "", false
		}
	}
	return value, true
}

func findBoundaryIndex(view *workspace.View, directories, markers []string) int {
	if len(markers) == 0 {
		return 0
	}
	for index := len(directories) - 1; index >= 0; index-- {
		for _, marker := range markers {
			exists, err := view.Exists(workspace.Join(directories[index], marker))
			if err == nil && exists {
				return index
			}
		}
	}
	return 0
}

func (r *resolver) includeContext(logical string, stack []string, depth int) {
	exists, err := r.view.Exists(logical)
	if err != nil {
		stub := repositoryStub(logical, "gemini-context", "hierarchical Gemini context candidate")
		r.resolution.Findings = append(r.resolution.Findings, provider.ReadFailureFinding(providerID, r.target, stub, err))
		return
	}
	if !exists {
		return
	}
	source, readErr := provider.ReadMarkdown(r.view, providerID, logical, "gemini-context", "hierarchical Gemini context from memory boundary to target", false)
	if readErr != nil {
		r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, source)
		r.resolution.Findings = append(r.resolution.Findings, provider.ReadFailureFinding(providerID, r.target, source, readErr))
		return
	}
	source.Scope.Base = r.projectRoot
	if strings.TrimSpace(source.Content) == "" {
		source.Scope.Status = "excluded"
		source.Scope.Reason = "Gemini skips empty context content when concatenating hierarchical memory"
		source.Reason = source.Scope.Reason
		r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, source)
		r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
			Code: "ACI003", Severity: "warning", Title: "Gemini context file is empty", Summary: source.Scope.Reason,
			Providers: []string{providerID}, Targets: []string{r.target}, Sources: []string{logical}, Confidence: "high",
		})
		return
	}
	r.resolution.IncludedSources = append(r.resolution.IncludedSources, source)
	r.includeImports(source, stack, depth)
}

func (r *resolver) includeImports(source agentconfig.Source, stack []string, depth int) {
	parsed := parser.ParseMarkdown(source.ID, []byte(source.Content), false)
	for _, imported := range geminiImports(parsed.Content) {
		if depth >= r.maxDepth {
			r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
				Code: "ACI024", Severity: "warning", Title: "Gemini import depth limit reached",
				Summary:   fmt.Sprintf("An import from %s was not expanded beyond %d levels.", source.DisplayPath, r.maxDepth),
				Providers: []string{providerID}, Targets: []string{r.target}, Sources: []string{source.DisplayPath}, Confidence: "high",
			})
			continue
		}
		logical, safe := r.resolveImport(source.LogicalPath, imported)
		if !safe {
			r.excludeExternalImport(source, "A Gemini context import resolves outside the selected project boundary and was excluded.")
			continue
		}
		if logical == source.LogicalPath || contains(stack, logical) {
			r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
				Code: "ACI025", Severity: "warning", Title: "Gemini import cycle detected",
				Summary:   "Import expansion stopped after encountering a previously active repository source.",
				Providers: []string{providerID}, Targets: []string{r.target}, Sources: []string{source.DisplayPath, logical}, Confidence: "high",
			})
			continue
		}
		exists, existsErr := r.view.Exists(logical)
		if existsErr != nil {
			stub := repositoryStub(logical, "gemini-import", "Gemini import target could not be inspected")
			r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, stub)
			r.resolution.Findings = append(r.resolution.Findings, provider.ReadFailureFinding(providerID, r.target, stub, existsErr))
			continue
		}
		if !exists {
			stub := repositoryStub(logical, "gemini-import", "Gemini import target is missing")
			stub.Scope.Status = "excluded"
			r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, stub)
			r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
				Code: "ACI026", Severity: "warning", Title: "Gemini import target is missing",
				Summary:   "An in-project Gemini context import refers to a file that does not exist.",
				Providers: []string{providerID}, Targets: []string{r.target}, Sources: []string{source.DisplayPath, logical}, Confidence: "high",
				Remediation: []string{"Create the imported file or remove the stale import."},
			})
			continue
		}
		importedSource, readErr := provider.ReadMarkdown(r.view, providerID, logical, "gemini-import", "file imported by Gemini context", false)
		if readErr != nil {
			r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, importedSource)
			r.resolution.Findings = append(r.resolution.Findings, provider.ReadFailureFinding(providerID, r.target, importedSource, readErr))
			continue
		}
		importedSource.Scope.Type = "import"
		importedSource.Scope.Base = r.projectRoot
		if strings.TrimSpace(importedSource.Content) == "" {
			importedSource.Scope.Status = "excluded"
			importedSource.Scope.Reason = "imported Gemini context is empty"
			importedSource.Reason = importedSource.Scope.Reason
			r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, importedSource)
			continue
		}
		r.resolution.IncludedSources = append(r.resolution.IncludedSources, importedSource)
		nextStack := append(append([]string(nil), stack...), source.LogicalPath)
		r.includeImports(importedSource, nextStack, depth+1)
	}
}

func (r *resolver) resolveImport(source, imported string) (string, bool) {
	normalized := strings.ReplaceAll(imported, "\\", "/")
	lower := strings.ToLower(normalized)
	if strings.HasPrefix(normalized, "~") || hasWindowsDrivePrefix(normalized) || strings.HasPrefix(lower, "file://") || strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return "", false
	}
	var logical string
	if path.IsAbs(normalized) || filepath.IsAbs(imported) {
		mapped, err := r.view.LogicalFromAbsolute(filepath.FromSlash(normalized))
		if err != nil {
			return "", false
		}
		logical = mapped
	} else {
		logical = parser.ResolveRelative(source, normalized)
	}
	if logical == ".." || strings.HasPrefix(logical, "../") || !withinRoot(r.projectRoot, logical) {
		return "", false
	}
	return logical, true
}

// geminiImports follows the v0.50.0 memory import processor: an import starts
// at an @ preceded by whitespace (or the start of a line), continues until
// whitespace, and starts with '.', '/', or an ASCII letter. Backtick and tilde
// code regions are ignored conservatively.
func geminiImports(content string) []string {
	var imports []string
	inFence := false
	var fence byte
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if marker, ok := fenceMarker(trimmed); ok {
			if !inFence {
				inFence, fence = true, marker
			} else if marker == fence {
				inFence = false
			}
			continue
		}
		if inFence {
			continue
		}
		for index := 0; index < len(line); {
			if line[index] == '`' {
				end := index + 1
				for end < len(line) && line[end] == '`' {
					end++
				}
				delimiter := line[index:end]
				if closeAt := strings.Index(line[end:], delimiter); closeAt >= 0 {
					index = end + closeAt + len(delimiter)
					continue
				}
				index = end
				continue
			}
			if line[index] != '@' || (index > 0 && !isGeminiWhitespace(line[index-1])) {
				index++
				continue
			}
			end := index + 1
			for end < len(line) && !isGeminiWhitespace(line[end]) {
				end++
			}
			candidate := line[index+1 : end]
			if len(candidate) > 0 && (candidate[0] == '.' || candidate[0] == '/' || isASCIILetter(candidate[0])) {
				imports = append(imports, candidate)
			}
			index = end
		}
	}
	return imports
}

func fenceMarker(trimmed string) (byte, bool) {
	if len(trimmed) < 3 || (trimmed[0] != '`' && trimmed[0] != '~') {
		return 0, false
	}
	if trimmed[1] == trimmed[0] && trimmed[2] == trimmed[0] {
		return trimmed[0], true
	}
	return 0, false
}

func isGeminiWhitespace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n'
}

func isASCIILetter(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func (r *resolver) excludeExternalImport(source agentconfig.Source, summary string) {
	stub := agentconfig.Source{
		ID: "gemini:external-import:" + source.ID, Origin: "user", DisplayPath: "<external-import>",
		Kind: "gemini-import", ContentVisibility: "hidden",
		Scope:  agentconfig.Scope{Type: "import", Status: "excluded", Reason: "outside workspace privacy boundary"},
		Reason: "external path redacted and not read",
	}
	r.resolution.ExcludedSources = append(r.resolution.ExcludedSources, stub)
	r.resolution.Findings = append(r.resolution.Findings, agentconfig.Finding{
		Code: "ACI005", Severity: "warning", Title: "External Gemini import was not scanned", Summary: summary,
		Providers: []string{providerID}, Targets: []string{r.target}, Sources: []string{source.DisplayPath, "<external-import>"}, Confidence: "high",
		Remediation: []string{"Keep commit-ready Gemini context imports inside the selected project boundary."},
	})
}

func repositoryStub(logical, kind, reason string) agentconfig.Source {
	return agentconfig.Source{
		ID: providerID + ":" + kind + ":" + logical, Origin: "repository", LogicalPath: logical, DisplayPath: logical,
		Kind: kind, ContentVisibility: "metadata-only",
		Scope: agentconfig.Scope{Type: "hierarchical", Base: ".", Status: "unknown", Reason: reason}, Reason: reason,
	}
}

func withinRoot(root, logical string) bool {
	return root == "." || logical == root || strings.HasPrefix(logical, root+"/")
}

func appendUnique(values []string, candidate string) []string {
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func hasWindowsDrivePrefix(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':'
}

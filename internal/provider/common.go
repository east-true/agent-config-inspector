package provider

import (
	"errors"
	"fmt"

	"github.com/east-true/agent-config-inspector/internal/parser"
	"github.com/east-true/agent-config-inspector/internal/workspace"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

func ReadMarkdown(view *workspace.View, providerID, logical, kind, reason string, stripComments bool) (agentconfig.Source, error) {
	return readMarkdown(view, providerID, logical, kind, reason, stripComments, false)
}

func ReadPlainMarkdown(view *workspace.View, providerID, logical, kind, reason string, stripComments bool) (agentconfig.Source, error) {
	return readMarkdown(view, providerID, logical, kind, reason, stripComments, true)
}

func readMarkdown(view *workspace.View, providerID, logical, kind, reason string, stripComments, preserveFrontmatter bool) (agentconfig.Source, error) {
	id := fmt.Sprintf("%s:%s:%s", providerID, kind, logical)
	file, err := view.Read(logical)
	source := agentconfig.Source{
		ID:                id,
		Origin:            "repository",
		LogicalPath:       logical,
		DisplayPath:       logical,
		Kind:              kind,
		ContentVisibility: "metadata-only",
		SizeBytes:         file.Size,
		Scope: agentconfig.Scope{
			Type: "hierarchical", Base: ".", Status: "applies", Reason: reason,
		},
		Reason: reason,
	}
	if err != nil {
		return source, err
	}
	var parsed parser.Parsed
	if preserveFrontmatter {
		parsed = parser.ParsePlainMarkdown(id, file.Bytes, stripComments)
	} else {
		parsed = parser.ParseMarkdown(id, file.Bytes, stripComments)
	}
	rawDigest := parser.RawContentDigest(file.Bytes)
	normalizedDigest := parser.ContentDigest(parsed.Normalized)
	source.RawDigest = &rawDigest
	source.NormalizedDigest = &normalizedDigest
	source.Scope.Patterns = append([]string(nil), parsed.Paths...)
	source.Content = parsed.Content
	source.NormalizedContent = parsed.Normalized
	source.Units = parsed.Units
	return source, nil
}

func ReadExternalMarkdown(providerID string, external ExternalSource, reason string, stripComments bool) agentconfig.Source {
	return readExternalMarkdown(providerID, external, reason, stripComments, false)
}

func ReadExternalPlainMarkdown(providerID string, external ExternalSource, reason string, stripComments bool) agentconfig.Source {
	return readExternalMarkdown(providerID, external, reason, stripComments, true)
}

func readExternalMarkdown(providerID string, external ExternalSource, reason string, stripComments, preserveFrontmatter bool) agentconfig.Source {
	id := fmt.Sprintf("%s:%s:%s", providerID, external.Kind, external.Label)
	var parsed parser.Parsed
	if preserveFrontmatter {
		parsed = parser.ParsePlainMarkdown(id, external.Content, stripComments)
	} else {
		parsed = parser.ParseMarkdown(id, external.Content, stripComments)
	}
	return agentconfig.Source{
		ID: id, Origin: "user", DisplayPath: external.Label, Kind: external.Kind, ContentVisibility: "hidden",
		Scope: agentconfig.Scope{Type: "always", Patterns: append([]string(nil), parsed.Paths...), Status: "applies", Reason: reason}, Reason: reason,
		Content: parsed.Content, NormalizedContent: parsed.Normalized, Units: parsed.Units,
	}
}

func ReadFailureFinding(providerID, target string, source agentconfig.Source, err error) agentconfig.Finding {
	code, severity, title := "ACI004", "warning", "Instruction source could not be read"
	summary := "A bounded repository instruction source could not be read."
	remediation := []string{"Check file permissions and retry."}
	if errors.Is(err, workspace.ErrOutsideWorkspace) || errors.Is(err, workspace.ErrInvalidPath) {
		code, severity, title = "ACI005", "error", "Instruction path escapes the workspace"
		summary = "The instruction path was rejected because it resolves outside the selected workspace."
		remediation = []string{"Keep repository instruction references inside the selected workspace."}
	} else if errors.Is(err, workspace.ErrSymlink) {
		code, severity, title = "ACI006", "warning", "Symlinked instruction source was not followed"
		summary = "The instruction source crosses a symlink while symlink traversal is disabled."
		remediation = []string{"Inspect the symlink manually or opt in to safe in-workspace symlink traversal."}
	} else if errors.Is(err, workspace.ErrTooLarge) {
		code, severity, title = "ACI007", "warning", "Instruction source exceeds the safety limit"
		summary = "The instruction source was not read because it exceeds --max-source-bytes."
		remediation = []string{"Raise --max-source-bytes only after reviewing the file size."}
	}
	return agentconfig.Finding{
		Code: code, Severity: severity, Title: title, Summary: summary, Providers: []string{providerID},
		Targets: []string{target}, Sources: []string{source.DisplayPath}, Confidence: "high", Remediation: remediation,
	}
}

func TokenEstimate(sources []agentconfig.Source) agentconfig.TokenEstimate {
	bytes := 0
	for _, source := range sources {
		bytes += len(source.Content)
	}
	return agentconfig.TokenEstimate{Method: "utf8-bytes-divided-by-4", Value: (bytes + 3) / 4}
}

func FinalizeResolution(resolution *agentconfig.Resolution) {
	if resolution.IncludedSources == nil {
		resolution.IncludedSources = []agentconfig.Source{}
	}
	if resolution.ExcludedSources == nil {
		resolution.ExcludedSources = []agentconfig.Source{}
	}
	if resolution.Findings == nil {
		resolution.Findings = []agentconfig.Finding{}
	}
	values := make([]string, 0, len(resolution.IncludedSources))
	for i := range resolution.IncludedSources {
		resolution.IncludedSources[i].Order = i + 1
		values = append(values, resolution.IncludedSources[i].NormalizedContent)
	}
	resolution.EffectiveDigest = parser.EffectiveDigest(values)
	resolution.TokenEstimate = TokenEstimate(resolution.IncludedSources)
	resolution.State = "complete"
	if len(resolution.IncludedSources) == 0 {
		resolution.Prediction = "predicted-empty"
	} else {
		resolution.Prediction = "predicted-effective"
	}
}

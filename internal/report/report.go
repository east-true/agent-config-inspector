package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

func WriteJSON(writer io.Writer, value agentconfig.Report) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func WriteText(writer io.Writer, value agentconfig.Report) error {
	workspaceDisplay := value.Request.Workspace
	if value.Request.WorkspaceLabel != "" {
		workspaceDisplay += " (label: " + value.Request.WorkspaceLabel + ")"
	}
	if _, err := fmt.Fprintf(writer, "Agent Config Inspector %s\nWorkspace: %s\n", value.Tool.Version, workspaceDisplay); err != nil {
		return err
	}
	for _, result := range value.Results {
		if _, err := fmt.Fprintf(writer, "\nTarget: %s\n\n%s\n  provider: %s\n  surface: %s\n  support: %s (%s)\n  project root: %s\n  state: %s\n  prediction: %s\n",
			result.Target, result.Provider.ID, result.Provider.Provider, result.Provider.Surface,
			result.Provider.Support, result.Provider.Depth, result.ProjectRoot, result.State, result.Prediction); err != nil {
			return err
		}
		if len(result.IncludedSources) == 0 {
			if _, err := fmt.Fprintln(writer, "  included: none"); err != nil {
				return err
			}
			if explanation := emptyDiscoveryExplanation(result.Findings); explanation != "" {
				if _, err := fmt.Fprintf(writer, "  empty reason: %s\n", explanation); err != nil {
					return err
				}
			}
		} else {
			if _, err := fmt.Fprintln(writer, "  included:"); err != nil {
				return err
			}
			for _, source := range result.IncludedSources {
				if _, err := fmt.Fprintf(writer, "    %d. %-32s %s\n", source.Order, source.DisplayPath, source.Scope.Reason); err != nil {
					return err
				}
			}
		}
		if len(result.ExcludedSources) > 0 {
			if _, err := fmt.Fprintln(writer, "  excluded:"); err != nil {
				return err
			}
			for _, source := range result.ExcludedSources {
				if _, err := fmt.Fprintf(writer, "    - %-32s %s\n", source.DisplayPath, source.Scope.Reason); err != nil {
					return err
				}
			}
		}
		digest := result.EffectiveDigest.Value
		if len(digest) > 12 {
			digest = digest[:12]
		}
		if _, err := fmt.Fprintf(writer, "  effective digest: sha256:%s\n  token estimate: %d (%s)\n",
			digest, result.TokenEstimate.Value, result.TokenEstimate.Method); err != nil {
			return err
		}
	}
	allEmpty := allResultsEmpty(value.Results)
	if len(value.Comparisons) > 0 && allEmpty {
		if _, err := fmt.Fprintln(writer, "\nComparisons\n  omitted from text because every selected provider is predicted-empty; structured comparisons remain available in JSON\n\nScope note: general repository files and unsupported product-state directories are outside this instruction scan"); err != nil {
			return err
		}
	} else if len(value.Comparisons) > 0 {
		if _, err := fmt.Fprintln(writer, "\nComparisons"); err != nil {
			return err
		}
		for _, comparison := range value.Comparisons {
			status := "different"
			if comparison.Equivalent {
				status = "equivalent-normalized-units"
			}
			if _, err := fmt.Fprintf(writer, "  %s: %s vs %s — %s (shared %d, only first %d, only second %d)\n",
				comparison.Target, comparison.Providers[0], comparison.Providers[1], status,
				comparison.SharedUnitCount, len(comparison.OnlyInFirst), len(comparison.OnlyInSecond)); err != nil {
				return err
			}
		}
	}
	visibleFindings := findingsExcept(value.Findings, "ACI001")
	if len(visibleFindings) > 0 {
		if _, err := fmt.Fprintln(writer, "\nFindings"); err != nil {
			return err
		}
		for _, finding := range visibleFindings {
			if _, err := fmt.Fprintf(writer, "  %-7s %-6s %s\n           %s\n",
				strings.ToUpper(finding.Severity), finding.Code, finding.Title, finding.Summary); err != nil {
				return err
			}
		}
	}
	contextStatus := "not scanned"
	if value.Privacy.UserContextScanned {
		contextStatus = "scanned with safe redaction"
	}
	_, err := fmt.Fprintf(writer, "\nResult: %s, not observed model compliance\nSensitive user context: %s\n", aggregatePrediction(value.Results), contextStatus)
	return err
}

func emptyDiscoveryExplanation(findings []agentconfig.Finding) string {
	for _, finding := range findings {
		if finding.Code == "ACI001" {
			return finding.Summary
		}
	}
	return ""
}

func findingsExcept(findings []agentconfig.Finding, code string) []agentconfig.Finding {
	result := make([]agentconfig.Finding, 0, len(findings))
	for _, finding := range findings {
		if finding.Code != code {
			result = append(result, finding)
		}
	}
	return result
}

func allResultsEmpty(results []agentconfig.Resolution) bool {
	return len(results) > 0 && aggregatePrediction(results) == "predicted-empty"
}

func aggregatePrediction(results []agentconfig.Resolution) string {
	effective, empty := 0, 0
	for _, result := range results {
		switch result.Prediction {
		case "predicted-effective":
			effective++
		case "predicted-empty":
			empty++
		}
	}
	switch {
	case effective > 0 && empty > 0:
		return "predicted-mixed"
	case effective > 0:
		return "predicted-effective"
	default:
		return "predicted-empty"
	}
}

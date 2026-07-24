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
	workspaceDisplay := "hidden (use --workspace-label to add an explicit label)"
	if value.Request.WorkspaceLabel != "" {
		workspaceDisplay = value.Request.WorkspaceLabel + " (explicit label)"
	}
	if _, err := fmt.Fprintf(writer, "Agent Config Inspector %s\nWorkspace: %s\nOutput: human-readable text · use --format json for complete safe structured data\n", value.Tool.Version, workspaceDisplay); err != nil {
		return err
	}
	providerNames := make(map[string]string, len(value.Results))
	currentTarget := ""
	for _, result := range value.Results {
		providerNames[result.Provider.ID] = result.Provider.Surface
		if result.Target != currentTarget {
			if _, err := fmt.Fprintf(writer, "\nTarget: %s\n", result.Target); err != nil {
				return err
			}
			currentTarget = result.Target
		}
		observation := "user context not scanned · runtime/model compliance not observed"
		if value.Privacy.UserContextScanned {
			observation = "opted-in user context included with redaction · runtime/model compliance not observed"
		}
		if _, err := fmt.Fprintf(writer, "\n%s\n  Result: %s\n  Provider: %s · ID: %s\n  Adapter: %s · %s\n  Scope: supported instruction files only; README, source, and product state excluded\n  Observation: %s\n",
			result.Provider.Surface, predictionDisplay(result.Prediction), result.Provider.Provider, result.Provider.ID,
			result.Provider.Support, result.Provider.Depth, observation); err != nil {
			return err
		}
		if result.State != "complete" {
			if _, err := fmt.Fprintf(writer, "  Analysis state: %s\n", result.State); err != nil {
				return err
			}
		}
		if len(result.IncludedSources) == 0 {
			if finding, ok := findingByCode(result.Findings, "ACI001"); ok {
				if _, err := fmt.Fprintf(writer, "  Why: %s\n", finding.Summary); err != nil {
					return err
				}
				if len(finding.Remediation) > 0 {
					if _, err := fmt.Fprintln(writer, "  Next steps:"); err != nil {
						return err
					}
					for _, remediation := range finding.Remediation {
						if _, err := fmt.Fprintf(writer, "    - %s\n", remediation); err != nil {
							return err
						}
					}
				}
			}
		} else {
			if _, err := fmt.Fprintln(writer, "  Instructions, in resolution order:"); err != nil {
				return err
			}
			for _, source := range result.IncludedSources {
				if _, err := fmt.Fprintf(writer, "    %d. %s\n       Why: %s\n", source.Order, source.DisplayPath, source.Scope.Reason); err != nil {
					return err
				}
			}
		}
		if len(result.ExcludedSources) > 0 {
			if _, err := fmt.Fprintln(writer, "  Not applied:"); err != nil {
				return err
			}
			for _, source := range result.ExcludedSources {
				if _, err := fmt.Fprintf(writer, "    - %s\n      Why: %s\n", source.DisplayPath, source.Scope.Reason); err != nil {
					return err
				}
			}
		}
		if result.Prediction == "predicted-effective" {
			fingerprint := shortFingerprint(result.EffectiveDigest)
			tokenEstimate := fmt.Sprintf("%d (%s)", result.TokenEstimate.Value, tokenMethodDisplay(result.TokenEstimate.Method))
			if result.TokenEstimate.Method == "redacted" {
				tokenEstimate = "hidden because user context was redacted"
			}
			if _, err := fmt.Fprintf(writer, "  Fingerprint: %s\n  Estimated tokens: %s\n", fingerprint, tokenEstimate); err != nil {
				return err
			}
		}
	}
	allEmpty := allResultsEmpty(value.Results)
	if len(value.Comparisons) > 0 && allEmpty {
		if _, err := fmt.Fprintln(writer, "\nComparisons\n  Not shown because every selected agent has no applicable instructions.\n  Use --format json for structured comparison data."); err != nil {
			return err
		}
	} else if len(value.Comparisons) > 0 {
		if _, err := fmt.Fprintln(writer, "\nComparisons"); err != nil {
			return err
		}
		for _, comparison := range value.Comparisons {
			status := "Different normalized instructions"
			if comparison.Equivalent {
				status = "Same normalized instructions"
			}
			first := providerDisplayName(providerNames, comparison.Providers[0])
			second := providerDisplayName(providerNames, comparison.Providers[1])
			if _, err := fmt.Fprintf(writer, "  Target %s · %s vs %s\n    %s\n    Shared: %d · %s only: %d · %s only: %d\n",
				comparison.Target, first, second, status, comparison.SharedUnitCount,
				first, len(comparison.OnlyInFirst), second, len(comparison.OnlyInSecond)); err != nil {
				return err
			}
		}
	}
	visibleFindings := findingsExcept(value.Findings, "ACI001")
	if _, err := fmt.Fprintln(writer, "\nFindings"); err != nil {
		return err
	}
	if len(visibleFindings) == 0 {
		_, err := fmt.Fprintln(writer, "  None.")
		return err
	}
	for _, finding := range visibleFindings {
		if _, err := fmt.Fprintf(writer, "  %s · %s · %s\n    %s\n",
			strings.ToUpper(finding.Severity), finding.Code, finding.Title, finding.Summary); err != nil {
			return err
		}
		if scope := findingScopeDisplay(finding, providerNames); scope != "" {
			if _, err := fmt.Fprintf(writer, "    Applies to: %s\n", scope); err != nil {
				return err
			}
		}
		if len(finding.Remediation) > 0 {
			if _, err := fmt.Fprintln(writer, "    Next steps:"); err != nil {
				return err
			}
			for _, remediation := range finding.Remediation {
				if _, err := fmt.Fprintf(writer, "      - %s\n", remediation); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func predictionDisplay(prediction string) string {
	switch prediction {
	case "predicted-effective":
		return "Applicable instructions found"
	case "predicted-empty":
		return "No applicable instructions"
	default:
		return prediction
	}
}

func shortFingerprint(digest agentconfig.Digest) string {
	if digest.Algorithm == "redacted" || digest.Value == "hidden" {
		return "hidden because user context was redacted"
	}
	value := digest.Value
	if len(value) > 12 {
		value = value[:12]
	}
	return digest.Algorithm + ":" + value
}

func tokenMethodDisplay(method string) string {
	if method == "utf8-bytes-divided-by-4" {
		return "UTF-8 bytes / 4"
	}
	return method
}

func providerDisplayName(names map[string]string, providerID string) string {
	if name := names[providerID]; name != "" {
		return name
	}
	return providerID
}

func findingScopeDisplay(finding agentconfig.Finding, providerNames map[string]string) string {
	parts := make([]string, 0, 2)
	if len(finding.Providers) > 0 {
		providers := make([]string, 0, len(finding.Providers))
		for _, providerID := range finding.Providers {
			providers = append(providers, providerDisplayName(providerNames, providerID))
		}
		parts = append(parts, strings.Join(providers, ", "))
	}
	if len(finding.Targets) > 0 {
		parts = append(parts, "target "+strings.Join(finding.Targets, ", "))
	}
	return strings.Join(parts, " · ")
}

func findingByCode(findings []agentconfig.Finding, code string) (agentconfig.Finding, bool) {
	for _, finding := range findings {
		if finding.Code == code {
			return finding, true
		}
	}
	return agentconfig.Finding{}, false
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

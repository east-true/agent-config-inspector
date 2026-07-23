package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/url"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

const sarifSchema = "https://json.schemastore.org/sarif-2.1.0.json"

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	ShortDescription sarifMessage         `json:"shortDescription"`
	HelpURI          string               `json:"helpUri"`
	Properties       *sarifRuleProperties `json:"properties,omitempty"`
}

type sarifRuleProperties struct {
	Tags            []string `json:"tags"`
	ProblemSeverity string   `json:"problem.severity"`
	Precision       string   `json:"precision"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	RuleIndex           int               `json:"ruleIndex"`
	Level               string            `json:"level"`
	Message             sarifMessage      `json:"message"`
	Locations           []sarifLocation   `json:"locations,omitempty"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
	Properties          sarifProperties   `json:"properties"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

type sarifProperties struct {
	Providers  []string `json:"providers,omitempty"`
	Targets    []string `json:"targets,omitempty"`
	Confidence string   `json:"confidence"`
}

func WriteSARIF(writer io.Writer, value agentconfig.Report) error {
	findings := append([]agentconfig.Finding(nil), value.Findings...)
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Code != findings[j].Code {
			return findings[i].Code < findings[j].Code
		}
		return findingFingerprint(findings[i]) < findingFingerprint(findings[j])
	})
	rulesByCode := make(map[string]agentconfig.Finding)
	for _, finding := range findings {
		if _, exists := rulesByCode[finding.Code]; !exists {
			rulesByCode[finding.Code] = finding
		}
	}
	codes := make([]string, 0, len(rulesByCode))
	for code := range rulesByCode {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	ruleIndexes := make(map[string]int, len(codes))
	rules := make([]sarifRule, 0, len(codes))
	for index, code := range codes {
		finding := rulesByCode[code]
		ruleIndexes[code] = index
		rules = append(rules, sarifRule{
			ID: code, Name: code, ShortDescription: sarifMessage{Text: finding.Title},
			HelpURI: "https://github.com/east-true/agent-config-inspector#accuracy-boundary",
			Properties: &sarifRuleProperties{
				Tags:            []string{"agent-configuration", "instruction-drift"},
				ProblemSeverity: finding.Severity, Precision: confidencePrecision(finding.Confidence),
			},
		})
	}
	results := make([]sarifResult, 0, len(findings))
	for _, finding := range findings {
		result := sarifResult{
			RuleID: finding.Code, RuleIndex: ruleIndexes[finding.Code], Level: sarifLevel(finding.Severity),
			Message:             sarifMessage{Text: finding.Title + ": " + finding.Summary},
			PartialFingerprints: map[string]string{"primaryLocationLineHash": findingFingerprint(finding)},
			Properties: sarifProperties{
				Providers: append([]string(nil), finding.Providers...), Targets: append([]string(nil), finding.Targets...), Confidence: finding.Confidence,
			},
		}
		for _, source := range finding.Sources {
			if !safeRepositoryLogicalPath(source) {
				continue
			}
			location := sarifLocation{PhysicalLocation: sarifPhysicalLocation{
				ArtifactLocation: sarifArtifactLocation{URI: (&url.URL{Path: source}).String(), URIBaseID: "%SRCROOT%"},
			}}
			if line := sourceStartLine(value, source); line > 0 {
				location.PhysicalLocation.Region = &sarifRegion{StartLine: line}
			}
			result.Locations = append(result.Locations, location)
		}
		results = append(results, result)
	}
	log := sarifLog{
		Schema: sarifSchema, Version: "2.1.0",
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name: value.Tool.Name, Version: value.Tool.Version,
				InformationURI: "https://github.com/east-true/agent-config-inspector", Rules: rules,
			}},
			Results: results,
		}},
	}
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(log)
}

func sarifLevel(severity string) string {
	switch severity {
	case "error":
		return "error"
	case "warning":
		return "warning"
	default:
		return "note"
	}
}

func confidencePrecision(confidence string) string {
	switch confidence {
	case "high":
		return "high"
	case "medium":
		return "medium"
	default:
		return "low"
	}
}

func findingFingerprint(finding agentconfig.Finding) string {
	parts := []string{finding.Code, strings.Join(finding.Providers, "\x1f"), strings.Join(finding.Targets, "\x1f"), strings.Join(finding.Sources, "\x1f")}
	sum := sha256.Sum256([]byte("agent-config-inspector/sarif-finding/v1\x00" + strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func safeRepositoryLogicalPath(logical string) bool {
	normalized := strings.ReplaceAll(logical, "\\", "/")
	if logical == "" || strings.HasPrefix(logical, "<") || path.IsAbs(normalized) || hasWindowsDrivePrefix(normalized) {
		return false
	}
	cleaned := path.Clean(normalized)
	if cleaned != normalized || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return false
	}
	for _, character := range logical {
		if character == 0 || unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func hasWindowsDrivePrefix(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':'
}

func sourceStartLine(report agentconfig.Report, logical string) int {
	for _, result := range report.Results {
		for _, source := range result.IncludedSources {
			if source.Origin == "repository" && source.LogicalPath == logical && len(source.Units) > 0 {
				return source.Units[0].StartLine
			}
		}
		for _, source := range result.ExcludedSources {
			if source.Origin == "repository" && source.LogicalPath == logical && len(source.Units) > 0 {
				return source.Units[0].StartLine
			}
		}
	}
	return 0
}

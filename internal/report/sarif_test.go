package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

func TestWriteSARIF(t *testing.T) {
	value := agentconfig.Report{
		Tool: agentconfig.ToolInfo{Name: "agent-config-inspector", Version: "test"},
		Results: []agentconfig.Resolution{{IncludedSources: []agentconfig.Source{{
			Origin: "repository", LogicalPath: "CLAUDE.md", Units: []agentconfig.Unit{{StartLine: 3}},
		}}}},
		Findings: []agentconfig.Finding{
			{Code: "ACI040", Severity: "warning", Title: "Drift", Summary: "Different instructions", Sources: []string{"CLAUDE.md"}, Confidence: "high"},
			{Code: "ACI005", Severity: "error", Title: "External", Summary: "Redacted", Sources: []string{"<external-import>", "/home/alice/private.md", `C:\Users\alice\private.md`}, Confidence: "high"},
		},
	}
	var output bytes.Buffer
	if err := WriteSARIF(&output, value); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if strings.Contains(text, "/home/alice") || strings.Contains(text, `C:\\Users`) || strings.Contains(text, "external-import") {
		t.Fatalf("external path leaked: %s", text)
	}
	var decoded struct {
		Version string `json:"version"`
		Runs    []struct {
			Results []struct {
				RuleID    string `json:"ruleId"`
				Locations []struct {
					PhysicalLocation struct {
						ArtifactLocation struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
						Region struct {
							StartLine int `json:"startLine"`
						} `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Version != "2.1.0" || len(decoded.Runs) != 1 || len(decoded.Runs[0].Results) != 2 {
		t.Fatalf("decoded = %#v", decoded)
	}
	var foundRepositoryLocation bool
	for _, result := range decoded.Runs[0].Results {
		if result.RuleID == "ACI040" && len(result.Locations) == 1 && result.Locations[0].PhysicalLocation.ArtifactLocation.URI == "CLAUDE.md" && result.Locations[0].PhysicalLocation.Region.StartLine == 3 {
			foundRepositoryLocation = true
		}
		if result.RuleID == "ACI005" && len(result.Locations) != 0 {
			t.Fatalf("external finding has locations: %#v", result.Locations)
		}
	}
	if !foundRepositoryLocation {
		t.Fatal("repository-relative location missing")
	}
}

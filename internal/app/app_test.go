package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	outputreport "github.com/east-true/agent-config-inspector/internal/report"
	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

func TestScanner(t *testing.T) {
	t.Run("both absent is equivalent without asymmetry", func(t *testing.T) {
		report, err := New().Scan(context.Background(), t.TempDir(), agentconfig.ScanOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Comparisons) != 3 || !allEquivalent(report.Comparisons) || hasCode(report.Findings, "ACI002") {
			t.Fatalf("report = %#v", report)
		}
	})
	t.Run("one sided instruction is reported", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, "AGENTS.md"), "Run tests")
		report, err := New().Scan(context.Background(), root, agentconfig.ScanOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !hasCode(report.Findings, "ACI002") || !hasCode(report.Findings, "ACI040") {
			t.Fatalf("findings = %#v", report.Findings)
		}
	})
	t.Run("equivalent native files share units", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, "AGENTS.md"), "Run tests")
		mustWrite(t, filepath.Join(root, "CLAUDE.md"), "Run tests")
		mustWrite(t, filepath.Join(root, "GEMINI.md"), "Run tests")
		report, err := New().Scan(context.Background(), root, agentconfig.ScanOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Comparisons) != 3 || !allEquivalent(report.Comparisons) {
			t.Fatalf("comparisons = %#v", report.Comparisons)
		}
	})
	for _, providerID := range []string{"kimi", "grok", "copilot"} {
		t.Run("unsupported provider "+providerID+" is explicit", func(t *testing.T) {
			_, err := New().Scan(context.Background(), t.TempDir(), agentconfig.ScanOptions{Providers: []string{providerID}})
			if err == nil || !strings.Contains(err.Error(), "unsupported provider") {
				t.Fatalf("err = %v", err)
			}
		})
	}
	t.Run("safe JSON is deterministic", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, "AGENTS.md"), "Run tests")
		first, err := New().Scan(context.Background(), root, agentconfig.ScanOptions{})
		if err != nil {
			t.Fatal(err)
		}
		second, err := New().Scan(context.Background(), root, agentconfig.ScanOptions{})
		if err != nil {
			t.Fatal(err)
		}
		firstJSON, _ := json.Marshal(first)
		secondJSON, _ := json.Marshal(second)
		if string(firstJSON) != string(secondJSON) {
			t.Fatalf("reports differ\n%s\n%s", firstJSON, secondJSON)
		}
	})
	t.Run("opted in user content is redacted", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		secret := "PRIVATE-INSTRUCTION-DO-NOT-LEAK"
		privatePattern := "private-customer-project/**"
		mustWrite(t, filepath.Join(home, ".claude", "CLAUDE.md"), secret)
		mustWrite(t, filepath.Join(home, ".claude", "rules", "private.md"), "---\npaths: [\""+privatePattern+"\"]\n---\nPrivate rule")
		report, err := New().Scan(context.Background(), t.TempDir(), agentconfig.ScanOptions{
			Providers: []string{"claude"}, IncludeUserContext: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		var outputBuffer bytes.Buffer
		if err := outputreport.WriteJSON(&outputBuffer, report); err != nil {
			t.Fatal(err)
		}
		output := outputBuffer.String()
		if strings.Contains(output, secret) || strings.Contains(output, privatePattern) || strings.Contains(output, home) || !strings.Contains(output, "<user-instruction-1>") || !strings.Contains(output, `"algorithm": "redacted"`) {
			t.Fatalf("unsafe output = %s", output)
		}
	})
	t.Run("opted in Gemini context and filename are redacted", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		secret := "PRIVATE-GEMINI-INSTRUCTION"
		privateFileName := "CUSTOM-PRIVATE-CONTEXT.md"
		mustWrite(t, filepath.Join(home, ".gemini", "settings.json"), `{"context":{"fileName":"`+privateFileName+`"}}`)
		mustWrite(t, filepath.Join(home, ".gemini", privateFileName), secret+"\n@private-import.md")
		report, err := New().Scan(context.Background(), t.TempDir(), agentconfig.ScanOptions{
			Providers: []string{"gemini"}, IncludeUserContext: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		var outputBuffer bytes.Buffer
		if err := outputreport.WriteJSON(&outputBuffer, report); err != nil {
			t.Fatal(err)
		}
		output := outputBuffer.String()
		if strings.Contains(output, secret) || strings.Contains(output, privateFileName) || strings.Contains(output, "private-import") || strings.Contains(output, home) || !strings.Contains(output, "<user-instruction-1>") || !strings.Contains(output, `"algorithm": "redacted"`) {
			t.Fatalf("unsafe output = %s", output)
		}
	})
}

func allEquivalent(comparisons []agentconfig.Comparison) bool {
	for _, comparison := range comparisons {
		if !comparison.Equivalent {
			return false
		}
	}
	return true
}

func hasCode(findings []agentconfig.Finding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func mustWrite(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

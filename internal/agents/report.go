package agents

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

func WriteJSON(writer io.Writer, report agentconfig.AgentInventoryReport) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func WriteText(writer io.Writer, report agentconfig.AgentInventoryReport) error {
	if _, err := fmt.Fprintf(writer, "Agent Config Inspector %s\nAgent inventory: repository only; descriptions and instructions hidden\n\n", report.Tool.Version); err != nil {
		return err
	}
	for _, result := range report.Results {
		if _, err := fmt.Fprintf(writer, "Target: %s\n\n%s\n  provider: %s\n  capability: %s\n  state: %s\n", result.Target, result.Provider.ID, result.Provider.Provider, result.Capability, result.State); err != nil {
			return err
		}
		if len(result.AvailableAgents) == 0 {
			if _, err := fmt.Fprintln(writer, "  available agents: none"); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintln(writer, "  available agents:"); err != nil {
				return err
			}
			for _, agent := range result.AvailableAgents {
				capabilities := "none"
				if len(agent.DeclaredCapabilities) > 0 {
					capabilities = strings.Join(agent.DeclaredCapabilities, ",")
				}
				if _, err := fmt.Fprintf(writer, "    %s\n      path: %s\n      scope base: %s\n      format: %s\n      metadata: %s; description: %t; instructions: %t\n      declared capabilities: %s\n      source digest: %s\n", agent.Name, agent.DisplayPath, agent.ScopeBase, agent.Format, agent.MetadataStatus, agent.DescriptionPresent, agent.InstructionsPresent, capabilities, shortDigest(agent.SourceDigest)); err != nil {
					return err
				}
			}
		}
		if len(result.ExcludedAgents) > 0 {
			if _, err := fmt.Fprintln(writer, "  excluded agents:"); err != nil {
				return err
			}
			for _, agent := range result.ExcludedAgents {
				if _, err := fmt.Fprintf(writer, "    %s — %s (%s)\n", agent.DisplayPath, agent.Reason, agent.MetadataStatus); err != nil {
					return err
				}
			}
		}
		if _, err := fmt.Fprintln(writer); err != nil {
			return err
		}
	}
	if len(report.Findings) > 0 {
		if _, err := fmt.Fprintln(writer, "Findings"); err != nil {
			return err
		}
		for _, finding := range report.Findings {
			if _, err := fmt.Fprintf(writer, "  %-7s %s %s\n           %s\n", finding.Severity, finding.Code, finding.Title, finding.Summary); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(writer); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(writer, "Agent descriptions and instructions: hidden\nSensitive user context: not scanned\nResult: predicted repository discovery, not observed delegation or execution")
	return err
}

func shortDigest(digest *agentconfig.Digest) string {
	if digest == nil {
		return "unavailable"
	}
	value := digest.Value
	if len(value) > 12 {
		value = value[:12]
	}
	return digest.Algorithm + ":" + value
}

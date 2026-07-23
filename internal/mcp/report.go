package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

func WriteJSON(writer io.Writer, report agentconfig.MCPInventoryReport) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func WriteText(writer io.Writer, report agentconfig.MCPInventoryReport) error {
	if _, err := fmt.Fprintf(writer, "Agent Config Inspector %s\nMCP inventory: repository only; connection and credential values hidden\n\n", report.Tool.Version); err != nil {
		return err
	}
	for _, result := range report.Results {
		if _, err := fmt.Fprintf(writer, "Target: %s\n\n%s\n  provider: %s\n  capability: %s\n  state: %s\n", result.Target, result.Provider.ID, result.Provider.Provider, result.Capability, result.State); err != nil {
			return err
		}
		if len(result.AvailableServers) == 0 {
			if _, err := fmt.Fprintln(writer, "  available servers: none"); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintln(writer, "  available servers:"); err != nil {
				return err
			}
			for _, server := range result.AvailableServers {
				if err := writeServer(writer, server); err != nil {
					return err
				}
			}
		}
		if len(result.ExcludedServers) > 0 {
			if _, err := fmt.Fprintln(writer, "  excluded servers:"); err != nil {
				return err
			}
			for _, server := range result.ExcludedServers {
				if _, err := fmt.Fprintf(writer, "    %s — %s (%s, %s)\n", server.Name, server.Reason, server.Status, server.DisplayPath); err != nil {
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
	_, err := fmt.Fprintln(writer, "Commands, arguments, URLs, environment/header names and values, tokens, OAuth details, tool names, and approval values: hidden\nSensitive user context: not scanned\nResult: static repository inventory; no server was started or contacted")
	return err
}

func writeServer(writer io.Writer, server agentconfig.MCPServerRecord) error {
	fields := "none"
	if len(server.DeclaredFields) > 0 {
		fields = strings.Join(server.DeclaredFields, ",")
	}
	sources := server.DisplayPath
	if len(server.ContributingSources) > 1 {
		sources = strings.Join(server.ContributingSources, ",")
	}
	_, err := fmt.Fprintf(writer, "    %s\n      sources: %s\n      scope base: %s\n      transport: %s; status: %s; enabled: %t; required: %t\n      local execution: %t; credential-bearing fields: %t\n      declared fields: %s\n      metadata digest: %s\n", server.Name, sources, server.ScopeBase, server.Transport, server.Status, server.Enabled, server.Required, server.Executable, server.CredentialFieldsPresent, fields, shortDigest(server.MetadataDigest))
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

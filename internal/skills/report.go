package skills

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

func WriteJSON(writer io.Writer, report agentconfig.SkillInventoryReport) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func WriteText(writer io.Writer, report agentconfig.SkillInventoryReport) error {
	if _, err := fmt.Fprintf(writer, "Agent Config Inspector %s\nSkill inventory: repository only; metadata content hidden\n\n", report.Tool.Version); err != nil {
		return err
	}
	for _, result := range report.Results {
		if _, err := fmt.Fprintf(writer, "Target: %s\n\n%s\n  provider: %s\n  capability: %s\n  state: %s\n", result.Target, result.Provider.ID, result.Provider.Provider, result.Capability, result.State); err != nil {
			return err
		}
		if len(result.AvailableSkills) == 0 {
			if _, err := fmt.Fprintln(writer, "  available skills: none"); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintln(writer, "  available skills:"); err != nil {
				return err
			}
			for _, skill := range result.AvailableSkills {
				if _, err := fmt.Fprintf(writer, "    %s\n      path: %s\n      scope base: %s\n      metadata: %s; description present: %t\n      source digest: %s\n", skill.Name, skill.DisplayPath, skill.ScopeBase, skill.MetadataStatus, skill.DescriptionPresent, shortDigest(skill.SourceDigest)); err != nil {
					return err
				}
			}
		}
		if len(result.ExcludedSkills) > 0 {
			if _, err := fmt.Fprintln(writer, "  excluded skills:"); err != nil {
				return err
			}
			for _, skill := range result.ExcludedSkills {
				if _, err := fmt.Fprintf(writer, "    %s — %s (%s)\n", skill.DisplayPath, skill.Reason, skill.MetadataStatus); err != nil {
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
	_, err := fmt.Fprintln(writer, "Skill descriptions and bodies: hidden\nSensitive user context: not scanned\nResult: predicted repository discovery, not observed activation or execution")
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

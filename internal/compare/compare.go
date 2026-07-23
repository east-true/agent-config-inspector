package compare

import (
	"fmt"
	"sort"

	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

func Pair(first, second agentconfig.Resolution) (agentconfig.Comparison, []agentconfig.Finding) {
	comparison := agentconfig.Comparison{
		Target: first.Target, Providers: []string{first.Provider.ID, second.Provider.ID},
	}
	firstUnits := unitSet(first.IncludedSources)
	secondUnits := unitSet(second.IncludedSources)
	for digest := range firstUnits {
		if _, ok := secondUnits[digest]; ok {
			comparison.SharedUnitCount++
		} else {
			comparison.OnlyInFirst = append(comparison.OnlyInFirst, digest)
		}
	}
	for digest := range secondUnits {
		if _, ok := firstUnits[digest]; !ok {
			comparison.OnlyInSecond = append(comparison.OnlyInSecond, digest)
		}
	}
	sort.Strings(comparison.OnlyInFirst)
	sort.Strings(comparison.OnlyInSecond)
	comparison.Equivalent = len(comparison.OnlyInFirst) == 0 && len(comparison.OnlyInSecond) == 0

	providers := []string{first.Provider.ID, second.Provider.ID}
	targets := []string{first.Target}
	var findings []agentconfig.Finding
	firstEmpty := repositorySourceCount(first) == 0
	secondEmpty := repositorySourceCount(second) == 0
	if firstEmpty != secondEmpty {
		findings = append(findings, agentconfig.Finding{
			Code: "ACI002", Severity: "warning", Title: "Repository instructions are asymmetric",
			Summary:   "At least one selected provider receives no repository instruction source for this target.",
			Providers: providers, Targets: targets, Confidence: "high",
			Remediation: []string{"Add the provider's native instruction entry point or explicitly import a shared instruction file."},
		})
	}
	if first.EffectiveDigest.Value != second.EffectiveDigest.Value {
		findings = append(findings, agentconfig.Finding{
			Code: "ACI040", Severity: "warning", Title: "Predicted effective instruction graphs differ",
			Summary:   fmt.Sprintf("The providers differ by %d and %d normalized units.", len(comparison.OnlyInFirst), len(comparison.OnlyInSecond)),
			Providers: providers, Targets: targets, Confidence: "high",
		})
	}
	if duplicateUnitCount(first.IncludedSources) > 0 || duplicateUnitCount(second.IncludedSources) > 0 {
		findings = append(findings, agentconfig.Finding{
			Code: "ACI041", Severity: "info", Title: "Duplicate normalized instructions detected",
			Summary:   "At least one provider receives the same normalized instruction unit more than once.",
			Providers: providers, Targets: targets, Confidence: "high",
		})
	}
	commandDifference := !sameSet(commandSet(first.IncludedSources), commandSet(second.IncludedSources))
	if commandDifference {
		findings = append(findings, agentconfig.Finding{
			Code: "ACI042", Severity: "warning", Title: "Command guidance differs",
			Summary:   "Inline command tokens appear in only one provider's effective repository instructions.",
			Providers: providers, Targets: targets, Confidence: "medium",
		})
	}
	firstProhibitions := prohibitionCount(first.IncludedSources)
	secondProhibitions := prohibitionCount(second.IncludedSources)
	prohibitionDifference := (firstProhibitions == 0) != (secondProhibitions == 0)
	if prohibitionDifference {
		findings = append(findings, agentconfig.Finding{
			Code: "ACI043", Severity: "warning", Title: "A prohibition may be one-sided",
			Summary:   "Prohibition-like wording was detected for only one provider. This is a conservative lexical signal.",
			Providers: providers, Targets: targets, Confidence: "low",
		})
	}
	if !comparison.Equivalent && !commandDifference && !prohibitionDifference {
		findings = append(findings, agentconfig.Finding{
			Code: "ACI044", Severity: "info", Title: "Semantic parity is unknown",
			Summary:   "Normalized units differ, but this offline scanner does not guess whether differently worded instructions mean the same thing.",
			Providers: providers, Targets: targets, Confidence: "high",
		})
	}
	return comparison, findings
}

func All(results []agentconfig.Resolution) ([]agentconfig.Comparison, []agentconfig.Finding) {
	byTarget := make(map[string][]agentconfig.Resolution)
	var targets []string
	for _, result := range results {
		if _, ok := byTarget[result.Target]; !ok {
			targets = append(targets, result.Target)
		}
		byTarget[result.Target] = append(byTarget[result.Target], result)
	}
	sort.Strings(targets)
	var comparisons []agentconfig.Comparison
	var findings []agentconfig.Finding
	for _, target := range targets {
		items := byTarget[target]
		sort.Slice(items, func(i, j int) bool { return items[i].Provider.ID < items[j].Provider.ID })
		for i := 0; i < len(items); i++ {
			for j := i + 1; j < len(items); j++ {
				comparison, pairFindings := Pair(items[i], items[j])
				comparisons = append(comparisons, comparison)
				findings = append(findings, pairFindings...)
			}
		}
	}
	SortFindings(findings)
	return comparisons, findings
}

func SortFindings(findings []agentconfig.Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Code != findings[j].Code {
			return findings[i].Code < findings[j].Code
		}
		left, right := "", ""
		if len(findings[i].Targets) > 0 {
			left = findings[i].Targets[0]
		}
		if len(findings[j].Targets) > 0 {
			right = findings[j].Targets[0]
		}
		return left < right
	})
}

func unitSet(sources []agentconfig.Source) map[string]struct{} {
	result := make(map[string]struct{})
	for _, source := range sources {
		for _, unit := range source.Units {
			result[unit.Digest.Value] = struct{}{}
		}
	}
	return result
}

func commandSet(sources []agentconfig.Source) map[string]struct{} {
	result := make(map[string]struct{})
	for _, source := range sources {
		for _, unit := range source.Units {
			if unit.Command != "" {
				result[unit.Command] = struct{}{}
			}
		}
	}
	return result
}

func sameSet(first, second map[string]struct{}) bool {
	if len(first) != len(second) {
		return false
	}
	for value := range first {
		if _, ok := second[value]; !ok {
			return false
		}
	}
	return true
}

func duplicateUnitCount(sources []agentconfig.Source) int {
	seen := make(map[string]int)
	for _, source := range sources {
		for _, unit := range source.Units {
			seen[unit.Digest.Value]++
		}
	}
	duplicates := 0
	for _, count := range seen {
		if count > 1 {
			duplicates += count - 1
		}
	}
	return duplicates
}

func prohibitionCount(sources []agentconfig.Source) int {
	count := 0
	for _, source := range sources {
		for _, unit := range source.Units {
			if unit.Prohibition {
				count++
			}
		}
	}
	return count
}

func repositorySourceCount(resolution agentconfig.Resolution) int {
	count := 0
	for _, source := range resolution.IncludedSources {
		if source.Origin == "repository" {
			count++
		}
	}
	return count
}

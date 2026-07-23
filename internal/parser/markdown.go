package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/east-true/agent-config-inspector/pkg/agentconfig"
)

type Parsed struct {
	Content    string
	Normalized string
	Paths      []string
	Imports    []string
	Units      []agentconfig.Unit
}

var codeSpan = regexp.MustCompile("`([^`\\n]+)`")

func ParseMarkdown(sourceID string, raw []byte, stripComments bool) Parsed {
	content := strings.ReplaceAll(string(raw), "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	content = strings.TrimPrefix(content, "\ufeff")
	paths, body := parseFrontmatter(content)
	if stripComments {
		body = stripHTMLComments(body)
	}
	imports := extractImports(body)
	units := extractUnits(sourceID, body)
	return Parsed{
		Content:    body,
		Normalized: normalizeContent(body),
		Paths:      paths,
		Imports:    imports,
		Units:      units,
	}
}

func parseFrontmatter(content string) ([]string, string) {
	if !strings.HasPrefix(content, "---\n") {
		return nil, content
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return nil, content
	}
	front := content[4 : 4+end]
	body := content[4+end+5:]
	lines := strings.Split(front, "\n")
	var paths []string
	inPaths := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "paths:") {
			inPaths = true
			inline := strings.TrimSpace(strings.TrimPrefix(trimmed, "paths:"))
			if strings.HasPrefix(inline, "[") && strings.HasSuffix(inline, "]") {
				for _, item := range strings.Split(strings.Trim(inline, "[]"), ",") {
					if value := trimYAMLScalar(item); value != "" {
						paths = append(paths, value)
					}
				}
				inPaths = false
			}
			continue
		}
		if inPaths && strings.HasPrefix(trimmed, "-") {
			if value := trimYAMLScalar(strings.TrimPrefix(trimmed, "-")); value != "" {
				paths = append(paths, value)
			}
			continue
		}
		if trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inPaths = false
		}
	}
	sort.Strings(paths)
	return unique(paths), body
}

func trimYAMLScalar(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'")
	return strings.TrimSpace(value)
}

func extractImports(content string) []string {
	var imports []string
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		line = codeSpan.ReplaceAllString(line, "")
		for i := 0; i < len(line); i++ {
			if line[i] != '@' || (i > 0 && isImportChar(line[i-1])) {
				continue
			}
			j := i + 1
			for j < len(line) && isImportChar(line[j]) {
				j++
			}
			if j == i+1 {
				continue
			}
			candidate := strings.TrimRight(line[i+1:j], ".,;:!?)\"]}")
			if strings.Contains(candidate, "/") || strings.Contains(candidate, ".") || candidate == "README" {
				imports = append(imports, candidate)
			}
			i = j - 1
		}
	}
	return stableUnique(imports)
}

func stripHTMLComments(content string) string {
	lines := strings.Split(content, "\n")
	inFence := false
	inComment := false
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		var output strings.Builder
		for position := 0; position < len(line); {
			if inComment {
				end := strings.Index(line[position:], "-->")
				if end < 0 {
					position = len(line)
					continue
				}
				inComment = false
				position += end + 3
				continue
			}
			start := strings.Index(line[position:], "<!--")
			if start < 0 {
				output.WriteString(line[position:])
				break
			}
			output.WriteString(line[position : position+start])
			inComment = true
			position += start + 4
		}
		lines[index] = output.String()
	}
	return strings.Join(lines, "\n")
}

func isImportChar(b byte) bool {
	return b == '/' || b == '\\' || b == '.' || b == '-' || b == '_' || b == '~' ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func extractUnits(sourceID, content string) []agentconfig.Unit {
	lines := strings.Split(content, "\n")
	units := make([]agentconfig.Unit, 0, len(lines))
	inFence := false
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		kind := "text"
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			kind = "code-fence"
		} else if inFence {
			kind = "code"
		} else if strings.HasPrefix(trimmed, "#") {
			kind = "heading"
		} else if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ") {
			kind = "list-item"
		}
		normalized := normalizeLine(trimmed, kind == "code")
		if normalized == "" {
			continue
		}
		digest := digestText("agent-config-inspector/unit/v1", normalized)
		command := ""
		if match := codeSpan.FindStringSubmatch(trimmed); len(match) == 2 {
			command = strings.TrimSpace(match[1])
		}
		lower := strings.ToLower(trimmed)
		prohibition := strings.Contains(lower, "never ") || strings.Contains(lower, "do not ") ||
			strings.Contains(lower, "must not ") || strings.Contains(trimmed, "금지") || strings.Contains(trimmed, "하지 마")
		units = append(units, agentconfig.Unit{
			Kind: kind, SourceID: sourceID, StartLine: index + 1, EndLine: index + 1,
			Digest: digest, Text: trimmed, Normalized: normalized, Command: command, Prohibition: prohibition,
		})
	}
	return units
}

func normalizeLine(line string, code bool) string {
	if code {
		return strings.TrimRight(line, " \t")
	}
	return strings.Join(strings.Fields(line), " ")
}

func normalizeContent(content string) string {
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	var compact []string
	blank := false
	for _, line := range lines {
		if line == "" {
			if blank {
				continue
			}
			blank = true
		} else {
			blank = false
		}
		compact = append(compact, line)
	}
	return strings.TrimSpace(strings.Join(compact, "\n"))
}

func digestText(domain, value string) agentconfig.Digest {
	sum := sha256.Sum256([]byte(domain + "\x00" + value))
	return agentconfig.Digest{Algorithm: "sha256", Value: hex.EncodeToString(sum[:])}
}

func ContentDigest(value string) agentconfig.Digest {
	return digestText("agent-config-inspector/repository-source/v1", value)
}

func RawContentDigest(value []byte) agentconfig.Digest {
	sum := sha256.Sum256(append([]byte("agent-config-inspector/repository-source/raw/v1\x00"), value...))
	return agentconfig.Digest{Algorithm: "sha256", Value: hex.EncodeToString(sum[:])}
}

func EffectiveDigest(values []string) agentconfig.Digest {
	return digestText("agent-config-inspector/effective-graph/v1", strings.Join(values, "\n\x00\n"))
}

func ResolveRelative(source, imported string) string {
	return path.Clean(path.Join(path.Dir(source), strings.ReplaceAll(imported, "\\", "/")))
}

func unique(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func stableUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

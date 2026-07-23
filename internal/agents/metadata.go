package agents

import (
	"errors"
	"sort"
	"strconv"
	"strings"
)

type parsedAgent struct {
	name                 string
	description          string
	instructions         string
	hasName              bool
	hasDescription       bool
	hasInstructions      bool
	declaredCapabilities []string
}

var claudeCapabilityFields = map[string]struct{}{
	"tools": {}, "disallowedTools": {}, "model": {}, "permissionMode": {}, "maxTurns": {},
	"skills": {}, "mcpServers": {}, "hooks": {}, "memory": {}, "background": {}, "effort": {},
	"isolation": {}, "color": {}, "initialPrompt": {},
}

var codexCapabilityFields = map[string]struct{}{
	"model": {}, "model_reasoning_effort": {}, "sandbox_mode": {}, "mcp_servers": {}, "skills": {}, "hooks": {}, "nickname_candidates": {},
}

func parseClaudeAgent(raw []byte) (parsedAgent, error) {
	content := normalize(string(raw))
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return parsedAgent{}, errors.New("agent Markdown must start with YAML frontmatter")
	}
	end := -1
	for index := 1; index < len(lines); index++ {
		if lines[index] == "---" {
			end = index
			break
		}
	}
	if end < 0 {
		return parsedAgent{}, errors.New("agent frontmatter is not terminated")
	}

	var result parsedAgent
	capabilities := make(map[string]struct{})
	for index := 1; index < end; index++ {
		line := lines[index]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if strings.TrimLeft(line, " \t") != line {
			continue
		}
		separator := strings.IndexByte(line, ':')
		if separator <= 0 {
			return parsedAgent{}, errors.New("frontmatter contains a malformed top-level field")
		}
		key := strings.TrimSpace(line[:separator])
		rawValue := strings.TrimSpace(line[separator+1:])
		if _, known := claudeCapabilityFields[key]; known {
			capabilities[key] = struct{}{}
		}
		if key != "name" && key != "description" {
			continue
		}
		if (key == "name" && result.hasName) || (key == "description" && result.hasDescription) {
			return parsedAgent{}, errors.New("frontmatter field " + key + " is repeated")
		}
		value := ""
		if isBlockIndicator(rawValue) {
			var next int
			value, next = parseYAMLBlock(lines, index+1, end, strings.HasPrefix(rawValue, ">"))
			index = next - 1
		} else {
			var err error
			value, err = parseYAMLScalar(rawValue)
			if err != nil {
				return parsedAgent{}, errors.New("frontmatter field " + key + ": " + err.Error())
			}
		}
		if key == "name" {
			result.name, result.hasName = value, true
		} else {
			result.description, result.hasDescription = value, true
		}
	}
	result.instructions = strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	result.hasInstructions = result.instructions != ""
	result.declaredCapabilities = sortedKeys(capabilities)
	return result, nil
}

func parseCodexAgent(raw []byte) (parsedAgent, error) {
	lines := strings.Split(normalize(string(raw)), "\n")
	var result parsedAgent
	capabilities := make(map[string]struct{})
	inTable := false
	for index := 0; index < len(lines); index++ {
		line := strings.TrimSpace(stripTOMLComment(lines[index]))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inTable = true
			table := strings.Trim(line, "[] ")
			root := strings.SplitN(table, ".", 2)[0]
			if _, known := codexCapabilityFields[root]; known {
				capabilities[root] = struct{}{}
			}
			continue
		}
		key, rawValue, found := strings.Cut(line, "=")
		if !found {
			// Unknown multiline values are outside the bounded metadata
			// projection. Required fields without '=' remain missing and are
			// rejected by the provider contract in inspectAgent.
			continue
		}
		key = strings.TrimSpace(key)
		if !inTable {
			if _, known := codexCapabilityFields[key]; known {
				capabilities[key] = struct{}{}
			}
		}
		if inTable || (key != "name" && key != "description" && key != "developer_instructions") {
			continue
		}
		if (key == "name" && result.hasName) || (key == "description" && result.hasDescription) || (key == "developer_instructions" && result.hasInstructions) {
			return parsedAgent{}, errors.New("TOML field " + key + " is repeated")
		}
		value, next, err := parseTOMLString(lines, index, strings.TrimSpace(rawValue))
		if err != nil {
			return parsedAgent{}, errors.New("TOML field " + key + ": " + err.Error())
		}
		index = next
		switch key {
		case "name":
			result.name, result.hasName = value, true
		case "description":
			result.description, result.hasDescription = value, true
		case "developer_instructions":
			result.instructions, result.hasInstructions = value, true
		}
	}
	result.declaredCapabilities = sortedKeys(capabilities)
	return result, nil
}

func parseTOMLString(lines []string, index int, value string) (string, int, error) {
	if strings.HasPrefix(value, `"""`) || strings.HasPrefix(value, "'''") {
		delimiter := value[:3]
		remainder := value[3:]
		if end := strings.Index(remainder, delimiter); end >= 0 {
			return remainder[:end], index, nil
		}
		values := []string{remainder}
		for next := index + 1; next < len(lines); next++ {
			if end := strings.Index(lines[next], delimiter); end >= 0 {
				values = append(values, lines[next][:end])
				return strings.TrimSpace(strings.Join(values, "\n")), next, nil
			}
			values = append(values, lines[next])
		}
		return "", index, errors.New("multiline string is not terminated")
	}
	if strings.HasPrefix(value, `"`) {
		unquoted, err := strconv.Unquote(value)
		if err != nil {
			return "", index, errors.New("has an invalid quoted string")
		}
		return unquoted, index, nil
	}
	if strings.HasPrefix(value, "'") {
		if len(value) < 2 || value[len(value)-1] != '\'' {
			return "", index, errors.New("has an unmatched quote")
		}
		return value[1 : len(value)-1], index, nil
	}
	return "", index, errors.New("must be a string")
}

func stripTOMLComment(value string) string {
	var quote byte
	for index := 0; index < len(value); index++ {
		character := value[index]
		if quote != 0 {
			if character == quote && (index == 0 || value[index-1] != '\\') {
				quote = 0
			}
			continue
		}
		if character == '\'' || character == '"' {
			quote = character
			continue
		}
		if character == '#' {
			return value[:index]
		}
	}
	return value
}

func isBlockIndicator(value string) bool {
	switch value {
	case "|", "|-", "|+", ">", ">-", ">+":
		return true
	default:
		return false
	}
}

func parseYAMLBlock(lines []string, start, end int, folded bool) (string, int) {
	minimumIndent := 0
	for index := start; index < end; index++ {
		if strings.TrimSpace(lines[index]) != "" {
			minimumIndent = leadingWhitespace(lines[index])
			break
		}
	}
	if minimumIndent == 0 {
		return "", start
	}
	var values []string
	index := start
	for ; index < end; index++ {
		line := lines[index]
		if strings.TrimSpace(line) != "" && leadingWhitespace(line) < minimumIndent {
			break
		}
		if len(line) >= minimumIndent {
			line = line[minimumIndent:]
		}
		values = append(values, line)
	}
	if !folded {
		return strings.TrimRight(strings.Join(values, "\n"), "\n"), index
	}
	var builder strings.Builder
	for item, line := range values {
		if item > 0 {
			if line == "" || values[item-1] == "" {
				builder.WriteByte('\n')
			} else {
				builder.WriteByte(' ')
			}
		}
		builder.WriteString(line)
	}
	return strings.TrimSpace(builder.String()), index
}

func parseYAMLScalar(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "[") || strings.HasPrefix(value, "{") {
		return "", errors.New("must be a scalar")
	}
	if value[0] == '\'' {
		if len(value) < 2 || value[len(value)-1] != '\'' {
			return "", errors.New("has an unmatched quote")
		}
		return strings.ReplaceAll(value[1:len(value)-1], "''", "'"), nil
	}
	if value[0] == '"' {
		unquoted, err := strconv.Unquote(value)
		if err != nil {
			return "", errors.New("has an invalid quoted scalar")
		}
		return unquoted, nil
	}
	if strings.Contains(value, ": ") {
		return "", errors.New("contains an unquoted colon")
	}
	return strings.TrimSpace(value), nil
}

func leadingWhitespace(value string) int {
	count := 0
	for count < len(value) && (value[count] == ' ' || value[count] == '\t') {
		count++
	}
	return count
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalize(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.TrimPrefix(value, "\ufeff")
}

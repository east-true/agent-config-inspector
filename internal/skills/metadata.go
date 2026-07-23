package skills

import (
	"errors"
	"strconv"
	"strings"
)

type metadata struct {
	name           string
	description    string
	hasName        bool
	hasDescription bool
}

func parseMetadata(raw []byte) (metadata, error) {
	content := strings.ReplaceAll(string(raw), "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	content = strings.TrimPrefix(content, "\ufeff")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return metadata{}, errors.New("SKILL.md must start with YAML frontmatter")
	}
	end := -1
	for index := 1; index < len(lines); index++ {
		if lines[index] == "---" {
			end = index
			break
		}
	}
	if end < 0 {
		return metadata{}, errors.New("SKILL.md frontmatter is not terminated")
	}

	var result metadata
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
			return metadata{}, errors.New("frontmatter contains a malformed top-level field")
		}
		key := strings.TrimSpace(line[:separator])
		rawValue := strings.TrimSpace(line[separator+1:])
		if key != "name" && key != "description" {
			continue
		}
		if (key == "name" && result.hasName) || (key == "description" && result.hasDescription) {
			return metadata{}, errors.New("frontmatter field " + key + " is repeated")
		}
		value := ""
		if isBlockIndicator(rawValue) {
			var next int
			value, next = parseBlock(lines, index+1, end, strings.HasPrefix(rawValue, ">"))
			index = next - 1
		} else {
			var err error
			value, err = parseInlineScalar(rawValue)
			if err != nil {
				return metadata{}, errors.New("frontmatter field " + key + ": " + err.Error())
			}
		}
		if key == "name" {
			result.name = value
			result.hasName = true
		} else {
			result.description = value
			result.hasDescription = true
		}
	}
	return result, nil
}

func isBlockIndicator(value string) bool {
	switch value {
	case "|", "|-", "|+", ">", ">-", ">+":
		return true
	default:
		return false
	}
}

func parseBlock(lines []string, start, end int, folded bool) (string, int) {
	index := start
	minimumIndent := 0
	for ; index < end; index++ {
		line := lines[index]
		if strings.TrimSpace(line) == "" {
			continue
		}
		minimumIndent = leadingWhitespace(line)
		break
	}
	if minimumIndent == 0 {
		return "", start
	}
	var values []string
	for index = start; index < end; index++ {
		line := lines[index]
		if strings.TrimSpace(line) != "" && leadingWhitespace(line) < minimumIndent {
			break
		}
		if len(line) >= minimumIndent {
			line = line[minimumIndent:]
		}
		values = append(values, line)
	}
	if folded {
		return foldBlock(values), index
	}
	return strings.TrimRight(strings.Join(values, "\n"), "\n"), index
}

func foldBlock(lines []string) string {
	var builder strings.Builder
	for index, line := range lines {
		if index > 0 {
			if line == "" || lines[index-1] == "" {
				builder.WriteByte('\n')
			} else {
				builder.WriteByte(' ')
			}
		}
		builder.WriteString(line)
	}
	return strings.TrimSpace(builder.String())
}

func leadingWhitespace(value string) int {
	count := 0
	for count < len(value) && (value[count] == ' ' || value[count] == '\t') {
		count++
	}
	return count
}

func parseInlineScalar(value string) (string, error) {
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

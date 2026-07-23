package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var claudeOutputFields = map[string]struct{}{
	"type": {}, "command": {}, "args": {}, "env": {}, "url": {}, "headers": {},
	"headersHelper": {}, "oauth": {}, "timeout": {}, "alwaysLoad": {},
}

var codexOutputFields = map[string]struct{}{
	"command": {}, "args": {}, "env": {}, "env_vars": {}, "cwd": {}, "experimental_environment": {},
	"url": {}, "auth": {}, "bearer_token_env_var": {}, "http_headers": {}, "env_http_headers": {},
	"startup_timeout_sec": {}, "startup_timeout_ms": {}, "tool_timeout_sec": {}, "enabled": {}, "required": {},
	"enabled_tools": {}, "disabled_tools": {}, "default_tools_approval_mode": {}, "tools": {}, "scopes": {},
	"oauth_resource": {}, "supports_parallel_tool_calls": {}, "environment_id": {}, "oauth": {},
}

var claudeReservedNames = map[string]struct{}{
	"workspace": {}, "claude-in-chrome": {}, "computer-use": {}, "Claude Preview": {}, "Claude Browser": {},
}

type valueState uint8

const (
	valueAbsent valueState = iota
	valueEmpty
	valuePresent
	valueInvalid
)

type serverFragment struct {
	name          string
	source        string
	scopeBase     string
	sourceSize    int64
	fields        map[string]struct{}
	command       valueState
	url           valueState
	enabled       *bool
	required      *bool
	invalid       bool
	invalidFields map[string]struct{}
}

func parseClaudeSource(raw []byte, source string, sourceSize int64) ([]serverFragment, error) {
	if err := validateJSON(raw); err != nil {
		return nil, errors.New("MCP JSON is malformed or contains repeated object fields")
	}
	var document struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, errors.New("MCP JSON is malformed")
	}
	if document.MCPServers == nil {
		return nil, errors.New("MCP JSON requires an mcpServers object")
	}
	fragments := make([]serverFragment, 0, len(document.MCPServers))
	for name, rawServer := range document.MCPServers {
		fragment := serverFragment{name: name, source: source, scopeBase: ".", sourceSize: sourceSize, fields: make(map[string]struct{}), invalidFields: make(map[string]struct{})}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(rawServer, &object); err != nil || object == nil {
			fragment.invalid = true
			fragments = append(fragments, fragment)
			continue
		}
		for key := range object {
			if _, ok := claudeOutputFields[key]; ok {
				fragment.fields[key] = struct{}{}
			}
		}
		fragment.command = jsonStringState(object, "command")
		fragment.url = jsonStringState(object, "url")
		fragments = append(fragments, fragment)
	}
	sort.Slice(fragments, func(i, j int) bool { return fragments[i].name < fragments[j].name })
	return fragments, nil
}

func jsonStringState(object map[string]json.RawMessage, key string) valueState {
	raw, exists := object[key]
	if !exists {
		return valueAbsent
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return valueInvalid
	}
	if strings.TrimSpace(value) == "" {
		return valueEmpty
	}
	return valuePresent
}

func claudeTransport(raw []byte) (string, valueState) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return "unknown", valueInvalid
	}
	rawType, exists := object["type"]
	if !exists {
		if jsonStringState(object, "command") != valueAbsent {
			return "stdio", valuePresent
		}
		return "unknown", valueAbsent
	}
	var transport string
	if err := json.Unmarshal(rawType, &transport); err != nil {
		return "unknown", valueInvalid
	}
	transport = strings.ToLower(strings.TrimSpace(transport))
	if transport == "streamable-http" {
		transport = "http"
	}
	switch transport {
	case "stdio", "http", "sse", "ws":
		return transport, valuePresent
	default:
		return "unknown", valueInvalid
	}
}

func validateJSON(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := walkJSONValue(decoder, 0); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("trailing JSON value")
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return errors.New("trailing JSON value")
	}
	return nil
}

func walkJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 128 {
		return errors.New("JSON nesting exceeds the safe limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("invalid object key")
			}
			if _, duplicate := seen[key]; duplicate {
				return errors.New("repeated object key")
			}
			seen[key] = struct{}{}
			if err := walkJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("unterminated object")
		}
	case '[':
		for decoder.More() {
			if err := walkJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("unterminated array")
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	return nil
}

// parseCodexSource reads only the bounded [mcp_servers.<name>] projection. It
// intentionally does not retain any TOML values except the booleans required
// to determine enabled/required state.
func parseCodexSource(raw []byte, source, scopeBase string, sourceSize int64) ([]serverFragment, error) {
	lines := strings.Split(normalize(string(raw)), "\n")
	fragments := make(map[string]*serverFragment)
	seenTables := make(map[string]struct{})
	seenAssignments := make(map[string]struct{})
	var table []string
	for index := 0; index < len(lines); index++ {
		line := strings.TrimSpace(stripTOMLComment(lines[index]))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[[") {
			if !strings.HasSuffix(line, "]]") {
				return nil, errors.New("MCP configuration contains an unterminated array table")
			}
			parts, err := parseDottedKey(strings.TrimSpace(line[2 : len(line)-2]))
			if err != nil {
				return nil, errors.New("MCP configuration contains an invalid array table name")
			}
			table = parts
			if len(parts) > 0 && parts[0] == "mcp_servers" {
				return nil, errors.New("MCP configuration does not support MCP array tables")
			}
			continue
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return nil, errors.New("MCP configuration contains an unterminated table")
			}
			parts, err := parseDottedKey(strings.TrimSpace(line[1 : len(line)-1]))
			if err != nil {
				return nil, errors.New("MCP configuration contains an invalid table name")
			}
			table = parts
			if len(parts) >= 2 && parts[0] == "mcp_servers" {
				canonical := strings.Join(parts, "\x00")
				if _, duplicate := seenTables[canonical]; duplicate {
					return nil, errors.New("MCP configuration repeats a table")
				}
				seenTables[canonical] = struct{}{}
				fragmentFor(fragments, parts[1], source, scopeBase, sourceSize)
			}
			continue
		}
		separator := assignmentSeparator(line)
		if separator < 1 {
			if len(table) >= 1 && table[0] == "mcp_servers" {
				return nil, errors.New("MCP configuration contains a malformed assignment")
			}
			continue
		}
		keyParts, err := parseDottedKey(strings.TrimSpace(line[:separator]))
		if err != nil {
			if len(table) >= 1 && table[0] == "mcp_servers" {
				return nil, errors.New("MCP configuration contains an invalid field name")
			}
			continue
		}
		full := append(append([]string{}, table...), keyParts...)
		if len(full) == 1 && full[0] == "mcp_servers" {
			return nil, errors.New("inline mcp_servers tables are outside the safe bounded parser")
		}
		if len(full) == 2 && full[0] == "mcp_servers" {
			return nil, errors.New("inline MCP server tables are outside the safe bounded parser")
		}
		if len(full) < 3 || full[0] != "mcp_servers" {
			continue
		}
		canonical := strings.Join(full, "\x00")
		if _, duplicate := seenAssignments[canonical]; duplicate {
			return nil, errors.New("MCP configuration repeats a field")
		}
		seenAssignments[canonical] = struct{}{}
		fragment := fragmentFor(fragments, full[1], source, scopeBase, sourceSize)
		field := full[2]
		if _, allowed := codexOutputFields[field]; allowed {
			fragment.fields[field] = struct{}{}
		}
		value := strings.TrimSpace(line[separator+1:])
		continuation, continuationErr := tomlValueEnd(lines, index, value)
		if continuationErr != nil {
			return nil, errors.New("MCP configuration contains an unterminated value")
		}
		switch {
		case len(full) == 3 && field == "command":
			fragment.command = tomlStringState(value)
		case len(full) == 3 && field == "url":
			fragment.url = tomlStringState(value)
		case len(full) == 3 && field == "enabled":
			parsed, ok := parseTOMLBool(value)
			if !ok {
				fragment.invalidFields[field] = struct{}{}
			} else {
				fragment.enabled = &parsed
				delete(fragment.invalidFields, field)
			}
		case len(full) == 3 && field == "required":
			parsed, ok := parseTOMLBool(value)
			if !ok {
				fragment.invalidFields[field] = struct{}{}
			} else {
				fragment.required = &parsed
				delete(fragment.invalidFields, field)
			}
		}
		index = continuation
	}
	result := make([]serverFragment, 0, len(fragments))
	for _, fragment := range fragments {
		result = append(result, *fragment)
	}
	return result, nil
}

func fragmentFor(values map[string]*serverFragment, name, source, scopeBase string, sourceSize int64) *serverFragment {
	if existing := values[name]; existing != nil {
		return existing
	}
	value := &serverFragment{name: name, source: source, scopeBase: scopeBase, sourceSize: sourceSize, fields: make(map[string]struct{}), invalidFields: make(map[string]struct{})}
	values[name] = value
	return value
}

func parseDottedKey(value string) ([]string, error) {
	var result []string
	for index := 0; index < len(value); {
		for index < len(value) && unicode.IsSpace(rune(value[index])) {
			index++
		}
		if index >= len(value) {
			return nil, errors.New("empty key")
		}
		var part string
		if value[index] == '\'' || value[index] == '"' {
			quote := value[index]
			start := index
			closed := false
			index++
			for index < len(value) {
				if value[index] == quote && (quote == '\'' || !isEscaped(value, index)) {
					index++
					closed = true
					break
				}
				index++
			}
			if !closed {
				return nil, errors.New("unterminated quoted key")
			}
			raw := value[start:index]
			if quote == '\'' {
				part = raw[1 : len(raw)-1]
			} else {
				decoded, err := strconv.Unquote(raw)
				if err != nil {
					return nil, err
				}
				part = decoded
			}
		} else {
			start := index
			for index < len(value) && value[index] != '.' && !unicode.IsSpace(rune(value[index])) {
				index++
			}
			part = value[start:index]
		}
		if part == "" {
			return nil, errors.New("empty key part")
		}
		result = append(result, part)
		for index < len(value) && unicode.IsSpace(rune(value[index])) {
			index++
		}
		if index == len(value) {
			break
		}
		if value[index] != '.' {
			return nil, errors.New("invalid dotted key")
		}
		index++
	}
	return result, nil
}

func assignmentSeparator(value string) int {
	var quote byte
	for index := 0; index < len(value); index++ {
		character := value[index]
		if quote != 0 {
			if character == quote && (quote == '\'' || !isEscaped(value, index)) {
				quote = 0
			}
			continue
		}
		if character == '\'' || character == '"' {
			quote = character
			continue
		}
		if character == '=' {
			return index
		}
	}
	return -1
}

func tomlStringState(value string) valueState {
	value = strings.TrimSpace(value)
	if len(value) < 2 {
		return valueInvalid
	}
	if strings.HasPrefix(value, `"""`) || strings.HasPrefix(value, "'''") {
		delimiter := value[:3]
		if value == delimiter+delimiter {
			return valueEmpty
		}
		return valuePresent
	}
	if value[0] == '\'' {
		if value[len(value)-1] != '\'' || len(value) == 2 {
			if len(value) == 2 {
				return valueEmpty
			}
			return valueInvalid
		}
		if strings.TrimSpace(value[1:len(value)-1]) == "" {
			return valueEmpty
		}
		return valuePresent
	}
	if value[0] != '"' {
		return valueInvalid
	}
	decoded, err := strconv.Unquote(value)
	if err != nil {
		return valueInvalid
	}
	if strings.TrimSpace(decoded) == "" {
		return valueEmpty
	}
	return valuePresent
}

func tomlValueEnd(lines []string, start int, value string) (int, error) {
	if strings.HasPrefix(value, `"""`) || strings.HasPrefix(value, "'''") {
		delimiter := value[:3]
		if strings.Contains(value[3:], delimiter) {
			return start, nil
		}
		for index := start + 1; index < len(lines); index++ {
			if strings.Contains(lines[index], delimiter) {
				return index, nil
			}
		}
		return start, errors.New("unterminated multiline string")
	}
	depth := 0
	var quote byte
	for index := start; index < len(lines); index++ {
		candidate := strings.TrimSpace(stripTOMLComment(lines[index]))
		if index == start {
			candidate = value
		}
		for offset := 0; offset < len(candidate); offset++ {
			character := candidate[offset]
			if quote != 0 {
				if character == quote && (quote == '\'' || !isEscaped(candidate, offset)) {
					quote = 0
				}
				continue
			}
			if character == '\'' || character == '"' {
				quote = character
				continue
			}
			switch character {
			case '[', '{':
				depth++
			case ']', '}':
				depth--
				if depth < 0 {
					return start, errors.New("unbalanced value")
				}
			}
		}
		if depth == 0 {
			return index, nil
		}
	}
	return start, errors.New("unterminated aggregate value")
}

func parseTOMLBool(value string) (bool, bool) {
	switch strings.TrimSpace(value) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func stripTOMLComment(value string) string {
	var quote byte
	for index := 0; index < len(value); index++ {
		character := value[index]
		if quote != 0 {
			if character == quote && (quote == '\'' || !isEscaped(value, index)) {
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

func normalize(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.TrimPrefix(value, "\ufeff")
}

func isEscaped(value string, index int) bool {
	backslashes := 0
	for index--; index >= 0 && value[index] == '\\'; index-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func safeServerName(value string) bool {
	return value != "" && len([]byte(value)) <= 128 && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

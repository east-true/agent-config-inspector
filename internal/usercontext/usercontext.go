package usercontext

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/east-true/agent-config-inspector/internal/provider"
)

type SafetyError struct{ message string }

func (e *SafetyError) Error() string { return e.message }

func Load(providerID string, maxSourceBytes int64) ([]provider.ExternalSource, error) {
	if maxSourceBytes <= 0 {
		maxSourceBytes = 1 << 20
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.New("cannot locate the user instruction directory")
	}
	switch providerID {
	case "anthropic-claude-code/cli":
		return loadClaude(home, maxSourceBytes)
	case "openai-codex/cli":
		return loadCodex(home, maxSourceBytes)
	case "google-gemini/cli":
		return loadGemini(home, maxSourceBytes)
	default:
		return nil, fmt.Errorf("user context is unsupported for provider %q", providerID)
	}
}

func loadGemini(home string, maxSourceBytes int64) ([]provider.ExternalSource, error) {
	root := filepath.Join(home, ".gemini")
	fileNames := []string{"GEMINI.md"}
	if content, ok, err := readBoundedUnder(root, filepath.Join(root, "settings.json"), maxSourceBytes); err != nil {
		return nil, err
	} else if ok {
		var raw struct {
			Context struct {
				FileName json.RawMessage `json:"fileName"`
			} `json:"context"`
		}
		if json.Unmarshal(content, &raw) == nil && len(raw.Context.FileName) > 0 {
			var configured []string
			var single string
			if json.Unmarshal(raw.Context.FileName, &single) == nil {
				configured = []string{single}
			} else {
				_ = json.Unmarshal(raw.Context.FileName, &configured)
			}
			var safe []string
			for _, name := range configured {
				if name, ok := safeDirectFilename(name); ok {
					safe = appendUnique(safe, name)
				}
			}
			if len(safe) > 0 {
				fileNames = appendUnique(safe, "GEMINI.md")
			}
		}
	}
	var result []provider.ExternalSource
	for _, name := range fileNames {
		content, ok, err := readBoundedUnder(root, filepath.Join(root, name), maxSourceBytes)
		if err != nil {
			return nil, err
		}
		if !ok || strings.TrimSpace(string(content)) == "" {
			continue
		}
		result = append(result, provider.ExternalSource{
			Label: fmt.Sprintf("<user-instruction-%d>", len(result)+1), Kind: "gemini-user-context", Content: content,
		})
	}
	return result, nil
}

func safeDirectFilename(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, "/\\") || filepath.Base(value) != value {
		return "", false
	}
	if len(value) >= 2 && value[1] == ':' {
		return "", false
	}
	for _, character := range value {
		if character == 0 || unicode.IsControl(character) {
			return "", false
		}
	}
	return value, true
}

func appendUnique(values []string, candidate string) []string {
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func loadClaude(home string, maxSourceBytes int64) ([]provider.ExternalSource, error) {
	root := filepath.Join(home, ".claude")
	var result []provider.ExternalSource
	if content, ok, err := readBoundedUnder(root, filepath.Join(root, "CLAUDE.md"), maxSourceBytes); err != nil {
		return nil, err
	} else if ok {
		result = append(result, provider.ExternalSource{Label: "<user-instruction-1>", Kind: "claude-user-memory", Content: content})
	}
	rulesRoot := filepath.Join(root, "rules")
	var rulePaths []string
	err := filepath.WalkDir(rulesRoot, func(candidate string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return filepath.SkipDir
		}
		if walkErr != nil {
			return errors.New("cannot enumerate a user instruction directory")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(candidate), ".md") {
			rulePaths = append(rulePaths, candidate)
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	sort.Strings(rulePaths)
	for index, rulePath := range rulePaths {
		content, ok, readErr := readBoundedUnder(root, rulePath, maxSourceBytes)
		if readErr != nil {
			return nil, readErr
		}
		if ok {
			result = append(result, provider.ExternalSource{
				Label: fmt.Sprintf("<user-rule-%d>", index+1), Kind: "claude-user-rule", Content: content,
			})
		}
	}
	return result, nil
}

func loadCodex(home string, maxSourceBytes int64) ([]provider.ExternalSource, error) {
	root := os.Getenv("CODEX_HOME")
	if root == "" {
		root = filepath.Join(home, ".codex")
	}
	for _, name := range []string{"AGENTS.override.md", "AGENTS.md"} {
		content, ok, err := readBoundedUnder(root, filepath.Join(root, name), maxSourceBytes)
		if err != nil {
			return nil, err
		}
		if ok && strings.TrimSpace(string(content)) != "" {
			return []provider.ExternalSource{{Label: "<user-instruction-1>", Kind: "codex-user-instruction", Content: content}}, nil
		}
	}
	return []provider.ExternalSource{}, nil
}

func readBounded(candidate string, maxSourceBytes int64) ([]byte, bool, error) {
	info, err := os.Lstat(candidate)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, errors.New("cannot inspect a user instruction source")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, false, &SafetyError{message: "user instruction symlinks are not followed"}
	}
	if !info.Mode().IsRegular() {
		return nil, false, nil
	}
	if info.Size() > maxSourceBytes {
		return nil, false, &SafetyError{message: "a user instruction source exceeds --max-source-bytes"}
	}
	content, err := os.ReadFile(candidate)
	if err != nil {
		return nil, false, errors.New("cannot read a user instruction source")
	}
	return content, true, nil
}

func readBoundedUnder(root, candidate string, maxSourceBytes int64) ([]byte, bool, error) {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, false, &SafetyError{message: "user instruction path escapes its documented directory"}
	}
	current := root
	paths := []string{current}
	if relative != "." {
		for _, component := range strings.Split(relative, string(filepath.Separator)) {
			current = filepath.Join(current, component)
			paths = append(paths, current)
		}
	}
	for _, checked := range paths {
		info, statErr := os.Lstat(checked)
		if errors.Is(statErr, os.ErrNotExist) {
			break
		}
		if statErr != nil {
			return nil, false, errors.New("cannot inspect a user instruction source")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, false, &SafetyError{message: "user instruction symlinks are not followed"}
		}
	}
	return readBounded(candidate, maxSourceBytes)
}

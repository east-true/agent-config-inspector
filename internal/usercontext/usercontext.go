package usercontext

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	default:
		return nil, fmt.Errorf("user context is unsupported for provider %q", providerID)
	}
}

func loadClaude(home string, maxSourceBytes int64) ([]provider.ExternalSource, error) {
	root := filepath.Join(home, ".claude")
	var result []provider.ExternalSource
	if content, ok, err := readBounded(filepath.Join(root, "CLAUDE.md"), maxSourceBytes); err != nil {
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
		content, ok, readErr := readBounded(rulePath, maxSourceBytes)
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
		content, ok, err := readBounded(filepath.Join(root, name), maxSourceBytes)
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

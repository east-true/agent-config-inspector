package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

var (
	ErrOutsideWorkspace = errors.New("path escapes workspace")
	ErrSymlink          = errors.New("symlink traversal is disabled")
	ErrTooLarge         = errors.New("source exceeds configured byte limit")
	ErrInvalidPath      = errors.New("invalid logical path")
)

type File struct {
	Logical string
	Bytes   []byte
	Size    int64
}

type View struct {
	root           string
	maxSourceBytes int64
	followSymlinks bool
}

func New(root string, maxSourceBytes int64, followSymlinks bool) (*View, error) {
	if maxSourceBytes <= 0 {
		maxSourceBytes = 1 << 20
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("stat workspace: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace is not a directory")
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace symlinks: %w", err)
	}
	return &View{root: filepath.Clean(real), maxSourceBytes: maxSourceBytes, followSymlinks: followSymlinks}, nil
}

func (v *View) Root() string { return v.root }

func (v *View) ProjectRootDisplay() string { return "<workspace>" }

func cleanLogical(logical string) (string, error) {
	if logical == "" || logical == "." {
		return ".", nil
	}
	if filepath.IsAbs(logical) || path.IsAbs(filepath.ToSlash(logical)) {
		return "", ErrInvalidPath
	}
	for _, r := range logical {
		if r == 0 || unicode.IsControl(r) {
			return "", ErrInvalidPath
		}
	}
	cleaned := path.Clean(strings.ReplaceAll(logical, "\\", "/"))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", ErrOutsideWorkspace
	}
	return cleaned, nil
}

func (v *View) resolve(logical string) (string, string, error) {
	cleaned, err := cleanLogical(logical)
	if err != nil {
		return "", "", err
	}
	abs := filepath.Join(v.root, filepath.FromSlash(cleaned))
	rel, err := filepath.Rel(v.root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", ErrOutsideWorkspace
	}
	if err := v.checkSymlinkPath(abs); err != nil {
		return "", "", err
	}
	return abs, cleaned, nil
}

func (v *View) checkSymlinkPath(abs string) error {
	rel, err := filepath.Rel(v.root, abs)
	if err != nil {
		return ErrOutsideWorkspace
	}
	current := v.root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if !v.followSymlinks {
				return ErrSymlink
			}
			real, err := filepath.EvalSymlinks(current)
			if err != nil {
				return err
			}
			realRel, err := filepath.Rel(v.root, real)
			if err != nil || realRel == ".." || strings.HasPrefix(realRel, ".."+string(filepath.Separator)) {
				return ErrOutsideWorkspace
			}
		}
	}
	return nil
}

func (v *View) NormalizeTarget(target string) (string, error) {
	abs, logical, err := v.resolve(target)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Stat(abs); statErr == nil && info.IsDir() {
		return logical, nil
	}
	return logical, nil
}

func (v *View) TargetDirectory(target string) (string, error) {
	abs, logical, err := v.resolve(target)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Stat(abs); statErr == nil && info.IsDir() {
		return logical, nil
	}
	dir := path.Dir(logical)
	if dir == "/" || dir == "" {
		dir = "."
	}
	return dir, nil
}

func (v *View) DirectoriesToTarget(target string) ([]string, error) {
	dir, err := v.TargetDirectory(target)
	if err != nil {
		return nil, err
	}
	if dir == "." {
		return []string{"."}, nil
	}
	parts := strings.Split(dir, "/")
	dirs := []string{"."}
	for i := range parts {
		dirs = append(dirs, strings.Join(parts[:i+1], "/"))
	}
	return dirs, nil
}

func (v *View) Exists(logical string) (bool, error) {
	abs, _, err := v.resolve(logical)
	if err != nil {
		return false, err
	}
	_, err = os.Lstat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (v *View) Read(logical string) (File, error) {
	abs, cleaned, err := v.resolve(logical)
	if err != nil {
		return File{}, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return File{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if !v.followSymlinks {
			return File{}, ErrSymlink
		}
		info, err = os.Stat(abs)
		if err != nil {
			return File{}, err
		}
	}
	if !info.Mode().IsRegular() {
		return File{}, fmt.Errorf("source is not a regular file")
	}
	if info.Size() > v.maxSourceBytes {
		return File{Logical: cleaned, Size: info.Size()}, ErrTooLarge
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return File{}, err
	}
	return File{Logical: cleaned, Bytes: b, Size: info.Size()}, nil
}

func (v *View) WalkFiles(logicalDir string, accept func(string, fs.DirEntry) bool) ([]string, error) {
	abs, cleaned, err := v.resolve(logicalDir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}
	var files []string
	err = filepath.WalkDir(abs, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == abs {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if !v.followSymlinks {
				return nil
			}
			rel, relErr := filepath.Rel(abs, p)
			if relErr != nil {
				return relErr
			}
			candidate := path.Join(cleaned, filepath.ToSlash(rel))
			if accept == nil || accept(candidate, entry) {
				files = append(files, candidate)
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(abs, p)
		if err != nil {
			return err
		}
		candidate := path.Join(cleaned, filepath.ToSlash(rel))
		if accept == nil || accept(candidate, entry) {
			files = append(files, candidate)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func Join(dir, name string) string {
	if dir == "." || dir == "" {
		return path.Clean(name)
	}
	return path.Join(dir, name)
}

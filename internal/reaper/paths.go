package reaper

import (
	"path/filepath"
	"runtime"
	"strings"
)

func pathInsideLexically(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func sameProjectPath(current, expected string) bool {
	currentCanonical, currentErr := filepath.EvalSymlinks(filepath.Clean(current))
	expectedCanonical, expectedErr := filepath.EvalSymlinks(filepath.Clean(expected))
	if currentErr != nil || expectedErr != nil {
		return false
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		return strings.EqualFold(currentCanonical, expectedCanonical)
	}
	return currentCanonical == expectedCanonical
}

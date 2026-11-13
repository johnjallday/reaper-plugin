//go:build !unix

package reaper

import (
	"errors"
	"os"
)

// openProjectFile refuses where O_NOFOLLOW is not available: the plugin ships
// for darwin/arm64 only, and a file that cannot be opened safely is failed.
func openProjectFile(string) (*os.File, error) {
	return nil, errors.New("project files are not read on this platform")
}

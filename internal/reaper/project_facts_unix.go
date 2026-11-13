//go:build unix

package reaper

import (
	"os"
	"syscall"
)

// openProjectFile opens path read-only with O_NOFOLLOW, so a symbolic link in
// the last element is refused rather than followed, and with O_NONBLOCK, so a
// FIFO or device named in place of a project cannot hold the call. The caller
// requires a regular file of the expected size before reading.
func openProjectFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
}

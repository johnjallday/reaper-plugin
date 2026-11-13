//go:build !unix

package reaper

import "testing"

func makeFIFO(t *testing.T, _ string) {
	t.Skip("project files are read only where O_NOFOLLOW exists")
}

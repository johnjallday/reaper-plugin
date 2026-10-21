//go:build !darwin

package reaper

import (
	"context"
	"os"
)

func (unsupportedPlatformProbe) OpenStudio(context.Context) *studioRoots {
	return &studioRoots{resourceState: StudioStateUnsupported, libraryState: StudioStateUnsupported}
}

func fileIdentity(os.FileInfo) (dev, ino uint64, ok bool) {
	return 0, 0, false
}

//go:build darwin

package reaper

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
)

// OpenStudio resolves the same fixed locations the profile read uses: the
// REAPER bundle in the system or the user's Applications folder, the resource
// folder of that installation (beside the bundle when a reaper.ini makes it
// portable, otherwise the per-user one), and the Ori script library. Every
// folder on the way is opened one element at a time and must be a real
// directory; nothing is created and no link is followed.
func (p *platformProbe) OpenStudio(_ context.Context) *studioRoots {
	roots := &studioRoots{supported: true, layout: StudioLayoutPerUser, resourceState: StudioStateAbsent, libraryState: StudioStateAbsent}
	if p == nil {
		roots.supported = false
		roots.resourceState, roots.libraryState = StudioStateUnsupported, StudioStateUnsupported
		return roots
	}
	home := p.openHome()
	defer closeRoot(home)
	homePath := ""
	if home != nil {
		homePath = filepath.Clean(home.Name())
	}
	roots.home = studioResolvedHome(homePath)

	applications, bundle := p.openInstallation(home)
	if bundle != nil {
		roots.installed = true
		roots.version = bundleVersion(bundle)
		closeRoot(bundle)
	}
	portable := applications != nil && regularFileIn(applications, "reaper.ini")
	if portable {
		roots.layout = StudioLayoutPortable
		roots.resource = applications
		roots.resourceState = StudioStateReady
		roots.resourcePath = filepath.Clean(applications.Name())
		roots.resourceDisplay = studioDisplayPath(roots.resourcePath, homePath)
	} else {
		closeRoot(applications)
		roots.resourcePath = filepath.Join(homePath, "Library", "Application Support", "REAPER")
		roots.resourceDisplay = "~/Library/Application Support/REAPER"
		roots.resource, roots.resourceState = openStudioFolder(home, "Library", "Application Support", "REAPER")
	}
	roots.identity = roots.resourcePath
	if roots.resource != nil {
		if info, err := roots.resource.Stat("."); err == nil {
			roots.dev, roots.ino, roots.hasIdentity = fileIdentity(info)
		}
	}

	roots.libraryPath = filepath.Join(homePath, "Ori Scripts", "reaper")
	roots.libraryDisplay = "~/Ori Scripts/reaper"
	roots.library, roots.libraryState = openStudioFolder(home, "Ori Scripts", "reaper")
	if homePath == "" {
		roots.resourceState, roots.libraryState = StudioStateDenied, StudioStateDenied
	}
	return roots
}

// studioResolvedHome is the home folder as Ori writes the paths it sends:
// with any link in it resolved. Only the home path itself is resolved, for
// display; nothing under it is followed.
func studioResolvedHome(home string) string {
	if home == "" {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(home)
	if err != nil {
		return filepath.Clean(home)
	}
	return filepath.Clean(resolved)
}

// fileIdentity is the file system's own identity of a directory: its device
// and inode numbers.
func fileIdentity(info os.FileInfo) (dev, ino uint64, ok bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return 0, 0, false
	}
	return uint64(stat.Dev), stat.Ino, true // #nosec G115 -- device numbers are small non-negative integers
}

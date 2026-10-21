//go:build darwin

package reaper

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// openStudioFolder opens elements below home one at a time and says what the
// last one is: ready (opened), absent (nothing of that name), unsupported (a
// link or another type) or denied (there, not openable).
func openStudioFolder(home *os.Root, elements ...string) (*os.Root, string) {
	if home == nil || len(elements) == 0 {
		return nil, StudioStateDenied
	}
	parent := openRealPath(home, elements[:len(elements)-1]...)
	if len(elements) > 1 {
		if parent == nil {
			return nil, studioMissingParentState(home, elements[:len(elements)-1])
		}
		defer closeRoot(parent)
	} else {
		parent = home
	}
	last := elements[len(elements)-1]
	info, err := parent.Lstat(last)
	switch {
	case err != nil && os.IsNotExist(err):
		return nil, StudioStateAbsent
	case err != nil:
		return nil, StudioStateDenied
	case info.Mode()&os.ModeSymlink != 0, !info.IsDir():
		return nil, StudioStateUnsupported
	}
	dir, _, ok := openRealDir(parent, last)
	if !ok {
		return nil, StudioStateDenied
	}
	return dir, StudioStateReady
}

// studioMissingParentState tells an absent parent folder from one that could
// not be used, walking the elements again without opening the last one.
func studioMissingParentState(home *os.Root, elements []string) string {
	current, owned := home, false
	defer func() {
		if owned {
			closeRoot(current)
		}
	}()
	for _, element := range elements {
		info, err := current.Lstat(element)
		switch {
		case err != nil && os.IsNotExist(err):
			return StudioStateAbsent
		case err != nil:
			return StudioStateDenied
		case info.Mode()&os.ModeSymlink != 0, !info.IsDir():
			return StudioStateUnsupported
		}
		next, _, ok := openRealDir(current, element)
		if !ok {
			return StudioStateDenied
		}
		if owned {
			closeRoot(current)
		}
		current, owned = next, true
	}
	return StudioStateDenied
}

// studioDisplayPath shows a path with "~" for the home folder.
func studioDisplayPath(path, home string) string {
	if home != "" && (path == home || strings.HasPrefix(path, home+string(filepath.Separator))) {
		return "~" + filepath.ToSlash(strings.TrimPrefix(path, home))
	}
	return filepath.ToSlash(path)
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

//go:build darwin

package reaper

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ReadProfile looks only at fixed platform locations: the application bundle
// in the system or the user's Applications folder, and the REAPER resource
// folder that goes with that bundle. No request, workspace or manifest value
// can add a location. Every folder on the way is opened one element at a time
// and must be a real directory, so no symbolic link is followed, not even one
// put in place while the read is running.
func (p *platformProbe) ReadProfile(_ context.Context, includeTemplates bool) ProfileResult {
	result := ProfileResult{App: profileAppName}
	if p == nil {
		return result
	}
	home := p.openHome()
	defer closeRoot(home)
	applications, bundle := p.openInstallation(home)
	if bundle == nil {
		return result
	}
	defer closeRoot(applications)
	defer closeRoot(bundle)
	result.Installed = true
	result.Version = bundleVersion(bundle)

	// The settings folder of the installation whose version is reported: the
	// folder the bundle is in when a reaper.ini sits beside it (a portable
	// install), otherwise the per-user one.
	resource := applications
	if !regularFileIn(applications, "reaper.ini") {
		resource = openRealPath(home, "Library", "Application Support", "REAPER")
		defer closeRoot(resource)
	}
	if !includeTemplates {
		result.TemplatesAvailable = profileTemplatesAvailable(resource)
		return result
	}
	result.Templates, result.TemplatesAvailable, result.Truncated = listProfileTemplates(resource)
	return result
}

func closeRoot(root *os.Root) {
	if root != nil {
		_ = root.Close()
	}
}

// openHome opens the user's home directory. It is the one place taken as
// given; everything below it is checked element by element.
func (p *platformProbe) openHome() *os.Root {
	if p.homeDir == nil {
		return nil
	}
	home, err := p.homeDir()
	if err != nil || strings.TrimSpace(home) == "" || !filepath.IsAbs(home) {
		return nil
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return nil
	}
	return root
}

// openRealPath opens elements below parent one at a time, each a real
// directory. It returns nil when any of them is missing, is a link or cannot
// be opened.
func openRealPath(parent *os.Root, elements ...string) *os.Root {
	current, owned := parent, false
	for _, element := range elements {
		next, _, ok := openRealDir(current, element)
		if owned {
			closeRoot(current)
		}
		if !ok {
			return nil
		}
		current, owned = next, true
	}
	if !owned {
		return nil
	}
	return current
}

// openSystemApplications opens the system Applications folder, a fixed
// location that must itself be a real directory.
func (p *platformProbe) openSystemApplications() *os.Root {
	system := p.systemApplications
	if system == "" {
		system = filepath.Join(string(filepath.Separator), "Applications")
	}
	before, err := os.Lstat(system) // #nosec G304 G703 -- fixed system location (a test fixture in tests)
	if err != nil || before.Mode()&os.ModeSymlink != 0 || !before.IsDir() {
		return nil
	}
	root, err := os.OpenRoot(system)
	if err != nil {
		return nil
	}
	after, err := root.Stat(".")
	if err != nil || !os.SameFile(before, after) {
		_ = root.Close()
		return nil
	}
	return root
}

// openInstallation returns the Applications folder that holds a real REAPER
// bundle and the bundle itself, the system one first. Both are nil when there
// is none.
func (p *platformProbe) openInstallation(home *os.Root) (applications, bundle *os.Root) {
	for _, candidate := range []*os.Root{p.openSystemApplications(), openRealPath(home, "Applications")} {
		if candidate == nil {
			continue
		}
		if bundle != nil {
			closeRoot(candidate)
			continue
		}
		if found, _, ok := openRealDir(candidate, reaperBundleName); ok {
			applications, bundle = candidate, found
			continue
		}
		closeRoot(candidate)
	}
	return applications, bundle
}

// bundleVersion reads the version from the bundle's own metadata file. A file
// that is missing, a link, not a regular file, too large or unreadable means
// no version. The file opened must be the one that was looked at, it is
// opened without waiting on anything, and no more than the bound is read.
func bundleVersion(bundle *os.Root) string {
	contents, _, ok := openRealDir(bundle, "Contents")
	if !ok {
		return ""
	}
	defer closeRoot(contents)
	const name = "Info.plist"
	before, err := contents.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || before.Size() > maxBundleInfoBytes {
		return ""
	}
	file, err := contents.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()
	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBundleInfoBytes+1))
	if err != nil || len(data) > maxBundleInfoBytes {
		return ""
	}
	return parseBundleVersion(data)
}

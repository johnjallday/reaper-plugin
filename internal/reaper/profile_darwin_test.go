//go:build darwin

package reaper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// profileMachine is a computer as the profile read sees it: a system
// Applications folder and a home directory, both temporary.
type profileMachine struct {
	t      *testing.T
	system string
	home   string
	probe  *platformProbe
}

func newProfileMachine(t *testing.T) *profileMachine {
	t.Helper()
	machine := &profileMachine{t: t, system: filepath.Join(t.TempDir(), "Applications"), home: t.TempDir()}
	if err := os.MkdirAll(machine.system, 0o700); err != nil {
		t.Fatal(err)
	}
	machine.probe = &platformProbe{
		homeDir:            func() (string, error) { return machine.home, nil },
		systemApplications: machine.system,
	}
	return machine
}

func (m *profileMachine) userResource() string {
	return filepath.Join(m.home, "Library", "Application Support", "REAPER")
}

// install puts a REAPER bundle with this version in an Applications folder.
func (m *profileMachine) install(applications, version string) string {
	m.t.Helper()
	bundle := filepath.Join(applications, reaperBundleName)
	if err := os.MkdirAll(filepath.Join(bundle, "Contents"), 0o700); err != nil {
		m.t.Fatal(err)
	}
	if version != "" {
		info := []byte(fmt.Sprintf(bundleInfoFixture, version))
		if err := os.WriteFile(filepath.Join(bundle, "Contents", "Info.plist"), info, 0o600); err != nil {
			m.t.Fatal(err)
		}
	}
	return bundle
}

func (m *profileMachine) read(includeTemplates bool) ProfileResult {
	return m.probe.ReadProfile(context.Background(), includeTemplates)
}

func TestReadProfileOnAComputerWithoutTheApplication(t *testing.T) {
	machine := newProfileMachine(t)
	// Templates left behind by an earlier installation are not reported.
	writeTemplate(t, filepath.Join(machine.userResource(), "ProjectTemplates"), "Old.RPP")
	facts := machine.read(true)
	if facts.App != "REAPER" || facts.Installed || facts.Version != "" || facts.TemplatesAvailable || facts.Templates != nil {
		t.Fatalf("facts = %+v", facts)
	}
	// A link named like the bundle is not an installation.
	elsewhere := filepath.Join(t.TempDir(), "Elsewhere.app")
	if err := os.MkdirAll(elsewhere, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(machine.system, reaperBundleName)); err != nil {
		t.Fatal(err)
	}
	if facts = machine.read(true); facts.Installed {
		t.Fatalf("a linked bundle counts as installed: %+v", facts)
	}
}

func TestReadProfileReportsVersionAndTheUsersTemplates(t *testing.T) {
	machine := newProfileMachine(t)
	machine.install(machine.system, "7.28.0_1a2b3c4d")
	writeTemplate(t, filepath.Join(machine.userResource(), "ProjectTemplates"), "Band Session.RPP")
	writeTemplate(t, filepath.Join(machine.userResource(), "TrackTemplates"), "Drum Bus.RTrackTemplate")

	// Without the request nothing in a template folder is named.
	facts := machine.read(false)
	if !facts.Installed || facts.Version != "7.28" || !facts.TemplatesAvailable || facts.Templates != nil || facts.Truncated {
		t.Fatalf("version-only facts = %+v", facts)
	}
	facts = machine.read(true)
	if got := templateNames(facts.Templates); len(got) != 2 || got[0] != "project:Band Session" || got[1] != "track:Drum Bus" {
		t.Fatalf("templates = %v", got)
	}
	if !facts.TemplatesAvailable || facts.Truncated || facts.Version != "7.28" {
		t.Fatalf("facts = %+v", facts)
	}
}

func TestReadProfileFindsAnApplicationInTheUsersOwnFolder(t *testing.T) {
	machine := newProfileMachine(t)
	machine.install(filepath.Join(machine.home, "Applications"), "6.83")
	if facts := machine.read(false); !facts.Installed || facts.Version != "6.83" || facts.TemplatesAvailable {
		t.Fatalf("facts = %+v", facts)
	}
	// The system installation is the one whose version is reported.
	machine.install(machine.system, "7.28")
	if facts := machine.read(false); facts.Version != "7.28" {
		t.Fatalf("facts = %+v", facts)
	}
}

func TestReadProfileWithoutUsableVersionMetadata(t *testing.T) {
	machine := newProfileMachine(t)
	bundle := machine.install(machine.system, "")
	if facts := machine.read(false); !facts.Installed || facts.Version != "" {
		t.Fatalf("no metadata: %+v", facts)
	}
	// Metadata that is a link is not read.
	outside := filepath.Join(t.TempDir(), "Info.plist")
	if err := os.WriteFile(outside, []byte(fmt.Sprintf(bundleInfoFixture, "9.99")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(bundle, "Contents", "Info.plist")); err != nil {
		t.Fatal(err)
	}
	if facts := machine.read(false); !facts.Installed || facts.Version != "" {
		t.Fatalf("linked metadata was read: %+v", facts)
	}
}

func TestReadProfileUsesThePortableFolderBesideTheApplication(t *testing.T) {
	machine := newProfileMachine(t)
	machine.install(machine.system, "7.28")
	writeTemplate(t, filepath.Join(machine.userResource(), "ProjectTemplates"), "Per User.RPP")
	writeTemplate(t, filepath.Join(machine.system, "ProjectTemplates"), "Portable.RPP")

	// Without a reaper.ini beside the bundle the installation is not portable.
	if got := templateNames(machine.read(true).Templates); len(got) != 1 || got[0] != "project:Per User" {
		t.Fatalf("per-user templates = %v", got)
	}
	if err := os.WriteFile(filepath.Join(machine.system, "reaper.ini"), []byte("[REAPER]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := templateNames(machine.read(true).Templates); len(got) != 1 || got[0] != "project:Portable" {
		t.Fatalf("portable templates = %v", got)
	}
}

func TestReadProfileNeverFollowsALinkedResourceFolder(t *testing.T) {
	machine := newProfileMachine(t)
	machine.install(machine.system, "7.28")
	elsewhere := t.TempDir()
	writeTemplate(t, filepath.Join(elsewhere, "ProjectTemplates"), "Elsewhere.RPP")
	support := filepath.Join(machine.home, "Library", "Application Support")
	if err := os.MkdirAll(support, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(support, "REAPER")); err != nil {
		t.Fatal(err)
	}
	facts := machine.read(true)
	if !facts.Installed || facts.TemplatesAvailable || facts.Templates != nil {
		t.Fatalf("a linked resource folder was followed: %+v", facts)
	}
	// A portable marker that is a link does not make the folder portable.
	if err := os.Symlink(filepath.Join(elsewhere, "missing.ini"), filepath.Join(machine.system, "reaper.ini")); err != nil {
		t.Fatal(err)
	}
	writeTemplate(t, filepath.Join(machine.system, "ProjectTemplates"), "Portable.RPP")
	if facts = machine.read(true); facts.TemplatesAvailable || facts.Templates != nil {
		t.Fatalf("a linked portable marker was trusted: %+v", facts)
	}
}

// Every folder on the way to the bundle and to the templates is a real
// directory or it is not used. A link at any of them leads nowhere, including
// the user's own Applications folder, whose contents would otherwise be read
// as a portable installation.
func TestReadProfileNeverFollowsALinkAnywhereOnTheWay(t *testing.T) {
	t.Run("the user's Applications folder", func(t *testing.T) {
		machine := newProfileMachine(t)
		elsewhere := t.TempDir()
		machine.install(elsewhere, "1.23")
		if err := os.WriteFile(filepath.Join(elsewhere, "reaper.ini"), []byte("[REAPER]\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		writeTemplate(t, filepath.Join(elsewhere, "ProjectTemplates"), "Secret.RPP")
		if err := os.Symlink(elsewhere, filepath.Join(machine.home, "Applications")); err != nil {
			t.Fatal(err)
		}
		if facts := machine.read(true); facts.Installed || facts.Version != "" || facts.Templates != nil {
			t.Fatalf("a linked Applications folder was followed: %+v", facts)
		}
	})
	for _, linked := range []string{"Library", "Library/Application Support"} {
		t.Run(linked, func(t *testing.T) {
			machine := newProfileMachine(t)
			machine.install(machine.system, "7.28")
			// The real tree lives elsewhere; the home only links to it.
			elsewhere := t.TempDir()
			rest, err := filepath.Rel(filepath.Join(machine.home, linked), machine.userResource())
			if err != nil {
				t.Fatal(err)
			}
			writeTemplate(t, filepath.Join(elsewhere, rest, "ProjectTemplates"), "Elsewhere.RPP")
			if err := os.MkdirAll(filepath.Dir(filepath.Join(machine.home, linked)), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(elsewhere, filepath.Join(machine.home, linked)); err != nil {
				t.Fatal(err)
			}
			facts := machine.read(true)
			if !facts.Installed || facts.TemplatesAvailable || facts.Templates != nil {
				t.Fatalf("a linked %s was followed: %+v", linked, facts)
			}
		})
	}
	t.Run("the bundle's Contents folder", func(t *testing.T) {
		machine := newProfileMachine(t)
		elsewhere := t.TempDir()
		if err := os.WriteFile(filepath.Join(elsewhere, "Info.plist"), []byte(fmt.Sprintf(bundleInfoFixture, "9.99")), 0o600); err != nil {
			t.Fatal(err)
		}
		bundle := filepath.Join(machine.system, reaperBundleName)
		if err := os.MkdirAll(bundle, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(elsewhere, filepath.Join(bundle, "Contents")); err != nil {
			t.Fatal(err)
		}
		if facts := machine.read(false); !facts.Installed || facts.Version != "" {
			t.Fatalf("linked bundle contents were read: %+v", facts)
		}
	})
	t.Run("the system Applications folder", func(t *testing.T) {
		machine := newProfileMachine(t)
		elsewhere := t.TempDir()
		machine.install(elsewhere, "7.28")
		if err := os.Remove(machine.system); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(elsewhere, machine.system); err != nil {
			t.Fatal(err)
		}
		if facts := machine.read(false); facts.Installed {
			t.Fatalf("a linked system Applications folder was followed: %+v", facts)
		}
	})
}

// The version and the templates describe one installation: the one that is
// reported. A second, portable copy elsewhere does not lend its templates.
func TestReadProfileDescribesOneInstallation(t *testing.T) {
	machine := newProfileMachine(t)
	machine.install(machine.system, "7.28")
	writeTemplate(t, filepath.Join(machine.userResource(), "ProjectTemplates"), "Per User.RPP")
	userApplications := filepath.Join(machine.home, "Applications")
	machine.install(userApplications, "6.10")
	if err := os.WriteFile(filepath.Join(userApplications, "reaper.ini"), []byte("[REAPER]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeTemplate(t, filepath.Join(userApplications, "ProjectTemplates"), "Old Portable.RPP")

	facts := machine.read(true)
	if got := templateNames(facts.Templates); facts.Version != "7.28" || len(got) != 1 || got[0] != "project:Per User" {
		t.Fatalf("version %q with templates %v", facts.Version, got)
	}
	// With the system copy gone, the portable one is the installation.
	if err := os.RemoveAll(filepath.Join(machine.system, reaperBundleName)); err != nil {
		t.Fatal(err)
	}
	facts = machine.read(true)
	if got := templateNames(facts.Templates); facts.Version != "6.10" || len(got) != 1 || got[0] != "project:Old Portable" {
		t.Fatalf("version %q with templates %v", facts.Version, got)
	}
}

// Version metadata that is not a small regular file is not read, and never
// waited on.
func TestReadProfileBoundsTheVersionMetadata(t *testing.T) {
	machine := newProfileMachine(t)
	bundle := machine.install(machine.system, "")
	info := filepath.Join(bundle, "Contents", "Info.plist")

	padded := fmt.Sprintf(bundleInfoFixture, "7.28") + strings.Repeat(" ", maxBundleInfoBytes)
	if err := os.WriteFile(info, []byte(padded), 0o600); err != nil {
		t.Fatal(err)
	}
	if facts := machine.read(false); !facts.Installed || facts.Version != "" {
		t.Fatalf("oversized metadata was read: %+v", facts)
	}
	if err := os.Remove(info); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(info, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan ProfileResult, 1)
	go func() { done <- machine.read(false) }()
	select {
	case facts := <-done:
		if !facts.Installed || facts.Version != "" {
			t.Fatalf("a pipe was read as metadata: %+v", facts)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the read waited on a pipe named like the metadata file")
	}
}

func TestReadProfileWithoutAHomeDirectory(t *testing.T) {
	machine := newProfileMachine(t)
	machine.install(machine.system, "7.28")
	machine.probe.homeDir = func() (string, error) { return "", os.ErrNotExist }
	if facts := machine.read(true); !facts.Installed || facts.Version != "7.28" || facts.TemplatesAvailable || facts.Templates != nil {
		t.Fatalf("facts = %+v", facts)
	}
	var none *platformProbe
	if facts := none.ReadProfile(context.Background(), true); facts.App != "REAPER" || facts.Installed {
		t.Fatalf("facts = %+v", facts)
	}
}

//go:build darwin

package reaper

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenStudioResolvesTheInstallationAndItsFoldersWithoutCreatingThem(t *testing.T) {
	machine := newProfileMachine(t)
	machine.install(machine.system, "7.80.0_9d9fa70u")
	writeTemplate(t, filepath.Join(machine.userResource(), "ProjectTemplates"), "Default.RPP")
	if err := os.MkdirAll(filepath.Join(machine.userResource(), "Scripts", "Cockos"), 0o700); err != nil {
		t.Fatal(err)
	}

	roots := machine.probe.OpenStudio(context.Background())
	defer roots.Close()
	if !roots.supported || !roots.installed || roots.version != "7.80" || roots.layout != StudioLayoutPerUser ||
		roots.resourceState != StudioStateReady || roots.resource == nil || !roots.hasIdentity ||
		roots.resourcePath != machine.userResource() || roots.resourceDisplay != "~/Library/Application Support/REAPER" ||
		roots.libraryState != StudioStateAbsent || roots.library != nil || roots.libraryPath != filepath.Join(machine.home, "Ori Scripts", "reaper") {
		t.Fatalf("roots = %+v", roots)
	}
	installation := roots.Installation()
	if installation.Label != "REAPER 7.80" || installation.AppVersion != "7.80" || !installation.AppInstalled || installation.Layout != StudioLayoutPerUser {
		t.Fatalf("installation = %+v", installation)
	}
	states := map[string]string{}
	for _, spec := range studioSourceSpecs {
		if spec.Access == StudioAccessFile {
			states[spec.ID], _ = roots.FolderState(spec)
		}
	}
	if states["project_templates"] != StudioStateReady || states["track_templates"] != StudioStateAbsent || states["reaper_scripts"] != StudioStateReady || states["ori_scripts"] != StudioStateAbsent {
		t.Fatalf("states = %v", states)
	}
	for _, absent := range []string{filepath.Join(machine.userResource(), "TrackTemplates"), filepath.Join(machine.home, "Ori Scripts")} {
		if _, err := os.Lstat(absent); !os.IsNotExist(err) {
			t.Fatalf("looking created %s", absent)
		}
	}

	// The id is stable for this folder and differs for another computer.
	again := machine.probe.OpenStudio(context.Background())
	defer again.Close()
	if again.InstallationID() != roots.InstallationID() || !strings.HasPrefix(roots.InstallationID(), "inst_") {
		t.Fatalf("installation id moved: %s vs %s", roots.InstallationID(), again.InstallationID())
	}
	other := newProfileMachine(t)
	other.install(other.system, "7.80")
	writeTemplate(t, filepath.Join(other.userResource(), "ProjectTemplates"), "Default.RPP")
	elsewhere := other.probe.OpenStudio(context.Background())
	defer elsewhere.Close()
	if elsewhere.InstallationID() == roots.InstallationID() {
		t.Fatal("another computer has the same installation id")
	}
}

func TestOpenStudioWithoutTheApplicationStillSeesTheFolders(t *testing.T) {
	machine := newProfileMachine(t)
	writeTemplate(t, filepath.Join(machine.userResource(), "ProjectTemplates"), "Old.RPP")
	roots := machine.probe.OpenStudio(context.Background())
	defer roots.Close()
	installation := roots.Installation()
	if roots.installed || installation.AppInstalled || installation.AppVersion != "" || installation.Label != "REAPER (not installed)" || roots.resourceState != StudioStateReady {
		t.Fatalf("roots = %+v, installation = %+v", roots, installation)
	}
}

func TestOpenStudioNeverFollowsALinkedFolder(t *testing.T) {
	machine := newProfileMachine(t)
	machine.install(machine.system, "7.80")
	elsewhere := t.TempDir()
	writeTemplate(t, filepath.Join(elsewhere, "ProjectTemplates"), "Elsewhere.RPP")
	support := filepath.Join(machine.home, "Library", "Application Support")
	if err := os.MkdirAll(support, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(support, "REAPER")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(machine.home, "Ori Scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(machine.home, "Ori Scripts", "reaper")); err != nil {
		t.Fatal(err)
	}
	roots := machine.probe.OpenStudio(context.Background())
	defer roots.Close()
	if roots.resourceState != StudioStateUnsupported || roots.resource != nil || roots.libraryState != StudioStateUnsupported || roots.library != nil {
		t.Fatalf("a linked folder was followed: %+v", roots)
	}
	spec, _ := lookupStudioSource("project_templates")
	if state, detail := roots.FolderState(spec); state != StudioStateUnsupported || detail == "" {
		t.Fatalf("state = %s %q", state, detail)
	}
}

func TestOpenStudioUsesThePortableFolderBesideTheApplication(t *testing.T) {
	machine := newProfileMachine(t)
	machine.install(machine.system, "7.80")
	writeTemplate(t, filepath.Join(machine.userResource(), "ProjectTemplates"), "Per User.RPP")
	writeTemplate(t, filepath.Join(machine.system, "ProjectTemplates"), "Portable.RPP")
	if err := os.WriteFile(filepath.Join(machine.system, "reaper.ini"), []byte("[REAPER]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	roots := machine.probe.OpenStudio(context.Background())
	defer roots.Close()
	if roots.layout != StudioLayoutPortable || roots.resourcePath != machine.system || roots.resourceState != StudioStateReady {
		t.Fatalf("roots = %+v", roots)
	}
	spec, _ := lookupStudioSource("project_templates")
	if location := roots.Location(spec); location != filepath.ToSlash(machine.system)+"/ProjectTemplates" {
		t.Fatalf("location = %q", location)
	}
	dir := roots.OpenSource(spec)
	if dir == nil {
		t.Fatal("portable source did not open")
	}
	defer closeRoot(dir)
	page, err := walkStudioSource(context.Background(), dir, spec, "", 256, 1<<20, nil)
	if err != nil || len(page.entries) != 1 || page.entries[0].Name != "Portable" {
		t.Fatalf("portable templates = %+v, %v", page, err)
	}
}

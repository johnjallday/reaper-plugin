package reaper

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// studioProbeStub opens a fresh view of a fixture for every call, the way the
// platform probe does; the service closes what it opened.
type studioProbeStub struct {
	build func() *studioRoots
}

func (s studioProbeStub) OpenStudio(context.Context) *studioRoots { return s.build() }

func writeStudioFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func openStudioDir(t *testing.T, path string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

// templateTree is a ProjectTemplates folder with the shapes a walk must handle:
// a sub-folder, a hidden file, a file of another type, two links, a decomposed
// name and a lower-case extension.
func templateTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeStudioFile(t, filepath.Join(root, "Default.RPP"), "<REAPER_PROJECT\n>\n")
	writeStudioFile(t, filepath.Join(root, "Zed.rpp"), "<REAPER_PROJECT\n>\n")
	writeStudioFile(t, filepath.Join(root, "Café.RPP"), "<REAPER_PROJECT\n>\n")
	writeStudioFile(t, filepath.Join(root, "Band", "Vocals.RPP"), "<REAPER_PROJECT\n>\n")
	writeStudioFile(t, filepath.Join(root, "Band", "Live tracking.RPP"), "<REAPER_PROJECT\n>\n")
	writeStudioFile(t, filepath.Join(root, "Band", ".hidden.RPP"), "<REAPER_PROJECT\n>\n")
	writeStudioFile(t, filepath.Join(root, "Band", "notes.txt"), "not a template")
	if err := os.Symlink(filepath.Join(root, "Default.RPP"), filepath.Join(root, "link.RPP")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "Band"), filepath.Join(root, "linkdir")); err != nil {
		t.Fatal(err)
	}
	return root
}

func studioIDs(entries []StudioEntry) []string {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.NativeID)
	}
	return ids
}

func TestStudioWalkListsFilesInWalkOrderAndCountsWhatItPassed(t *testing.T) {
	spec, _ := lookupStudioSource("project_templates")
	dir := openStudioDir(t, templateTree(t))
	page, err := walkStudioSource(context.Background(), dir, spec, "", 256, 1<<20, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Band/Live tracking.RPP", "Band/Vocals.RPP", "Café.RPP", "Default.RPP", "Zed.rpp"}
	if got := studioIDs(page.entries); !slices.Equal(got, want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	for index, name := range []string{"Live tracking", "Vocals", "Café", "Default", "Zed"} {
		entry := page.entries[index]
		if entry.Name != name || entry.Kind != StudioKindProjectTemplate || entry.ModifiedAt == "" || entry.Facts == nil || len(entry.Facts) != 0 || entry.Description != nil {
			t.Fatalf("entry %d = %+v", index, entry)
		}
	}
	if page.nextCursor != "" {
		t.Fatalf("one page holds everything, got cursor %q", page.nextCursor)
	}
	// Band, .hidden, two entries, notes.txt, three entries, two links.
	wantCoverage := StudioCoverage{Visited: 10, Skipped: []StudioSkip{{Reason: studioSkipHidden, Count: 1}, {Reason: studioSkipSymlink, Count: 2}}}
	if !reflect.DeepEqual(page.coverage, wantCoverage) {
		t.Fatalf("coverage = %+v, want %+v", page.coverage, wantCoverage)
	}
}

// Following cursors page by page lists exactly the one-shot entries, and the
// page coverages add up to the one-shot coverage: nothing is counted twice or
// lost at a page boundary, whether the page ends on the limit or on the byte
// budget.
func TestStudioWalkPagesAddUpToTheWholeWalk(t *testing.T) {
	spec, _ := lookupStudioSource("project_templates")
	dir := openStudioDir(t, templateTree(t))
	whole, err := walkStudioSource(context.Background(), dir, spec, "", 256, 1<<20, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name          string
		limit, budget int
	}{
		{"limit of two", 2, 1 << 20},
		{"limit of one", 1, 1 << 20},
		{"budget for one entry", 256, 160},
		{"budget for two entries", 256, 300},
	} {
		t.Run(test.name, func(t *testing.T) {
			var entries []StudioEntry
			visited, skips, cursor, pages := 0, map[string]int{}, "", 0
			for {
				pages++
				if pages > 10 {
					t.Fatal("paging does not end")
				}
				page, err := walkStudioSource(context.Background(), dir, spec, cursor, test.limit, test.budget, nil)
				if err != nil {
					t.Fatal(err)
				}
				entries = append(entries, page.entries...)
				visited += page.coverage.Visited
				for _, skip := range page.coverage.Skipped {
					skips[skip.Reason] += skip.Count
				}
				if page.nextCursor == "" {
					break
				}
				decoded, ok := decodeStudioCursor(page.nextCursor)
				if !ok || decoded != page.entries[len(page.entries)-1].NativeID {
					t.Fatalf("cursor %q does not name the last entry", page.nextCursor)
				}
				cursor = decoded
			}
			if !reflect.DeepEqual(entries, whole.entries) {
				t.Fatalf("paged entries = %v, want %v", studioIDs(entries), studioIDs(whole.entries))
			}
			if visited != whole.coverage.Visited || !reflect.DeepEqual(studioSkipList(skips), whole.coverage.Skipped) {
				t.Fatalf("paged coverage = %d %v, want %+v", visited, skips, whole.coverage)
			}
			if pages < 2 {
				t.Fatalf("expected more than one page, got %d", pages)
			}
		})
	}
}

func TestStudioWalkBoundsDepthAndFolderSize(t *testing.T) {
	spec, _ := lookupStudioSource("reaper_scripts")
	root := t.TempDir()
	deep := root
	for level := 1; level <= studioWalkDepthLimit; level++ {
		deep = filepath.Join(deep, fmt.Sprintf("d%02d", level))
	}
	writeStudioFile(t, filepath.Join(deep, "too-deep.lua"), "return 1\n")
	writeStudioFile(t, filepath.Join(root, "d01", "fine.lua"), "return 1\n")
	for index := 0; index < studioDirectoryEntryLimit+3; index++ {
		writeStudioFile(t, filepath.Join(root, "Many", fmt.Sprintf("n%05d.txt", index)), "x")
	}
	page, err := walkStudioSource(context.Background(), openStudioDir(t, root), spec, "", 256, 1<<20, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := studioIDs(page.entries); !slices.Equal(got, []string{"d01/fine.lua"}) {
		t.Fatalf("entries = %v", got)
	}
	skips := map[string]int{}
	for _, skip := range page.coverage.Skipped {
		skips[skip.Reason] = skip.Count
	}
	if skips[studioSkipDepthLimit] != 1 || skips[studioSkipTooLarge] != 3 || len(skips) != 2 {
		t.Fatalf("skips = %v", skips)
	}
	// d01..d15 folders, fine.lua, the folder at the depth limit, Many, its
	// 4096 looked-at names and the 3 beyond the bound.
	if page.coverage.Visited != 15+1+1+1+studioDirectoryEntryLimit+3 {
		t.Fatalf("visited = %d", page.coverage.Visited)
	}
}

func TestStudioWalkStopsWhenTheContextIsCancelled(t *testing.T) {
	spec, _ := lookupStudioSource("project_templates")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := walkStudioSource(ctx, openStudioDir(t, templateTree(t)), spec, "", 256, 1<<20, nil); err == nil {
		t.Fatal("a cancelled walk returned a page")
	}
}

func TestStudioCursorRoundTripsAndRefusesWhatItCannotHonour(t *testing.T) {
	for _, id := range []string{"Cockos/Lyrics.lua", "ReaTeam Scripts/Items/Split at grid.eel", "Café 100%.RPP", "a+b~c.rpp", "VST3:WaveShell1-VST3 16.7.vst3<1189814795"} {
		cursor := encodeStudioCursor(id)
		if strings.ContainsAny(cursor, " <>\"'\t\n") || len(cursor) > maxStudioCursorBytes {
			t.Fatalf("cursor %q is not plain", cursor)
		}
		decoded, ok := decodeStudioCursor(cursor)
		if !ok || decoded != id {
			t.Fatalf("cursor %q decoded to %q, %v", cursor, decoded, ok)
		}
	}
	if got := encodeStudioCursor("ReaTeam Scripts/Items/Split at grid.eel"); got != "after:ReaTeam%20Scripts/Items/Split%20at%20grid.eel" {
		t.Fatalf("cursor = %q", got)
	}
	for _, cursor := range []string{"after a", "at-0", "after:", "after:%zz", "after:a%2", "after:" + strings.Repeat("a", 600), "after:a%00b", "x"} {
		if _, ok := decodeStudioCursor(cursor); ok {
			t.Errorf("cursor %q was honoured", cursor)
		}
	}
	if decoded, ok := decodeStudioCursor(""); !ok || decoded != "" {
		t.Fatal("the empty cursor is the first page")
	}
	if compareStudioPath("a/b.lua", "a.lua") >= 0 || compareStudioPath("a", "a/b.lua") >= 0 || compareStudioPath("b", "a/z") <= 0 || compareStudioPath("x", "x") != 0 {
		t.Fatal("path order is not the walk order")
	}
}

// scriptsFixture is a REAPER resource folder with a Scripts tree, a keyboard
// configuration registering some of them, and a ReaPack remote list.
func scriptsFixture(t *testing.T) (resource string, roots func() *studioRoots) {
	t.Helper()
	resource = t.TempDir()
	library := filepath.Join(t.TempDir(), "Ori Scripts", "reaper")
	scripts := filepath.Join(resource, "Scripts")
	writeStudioFile(t, filepath.Join(scripts, "Cockos", "lyrics.lua"), "-- @description Shows the lyrics of the current item.\n-- @author Cockos\nlocal x = 1\n")
	writeStudioFile(t, filepath.Join(scripts, "Vocals", "lyrics.lua"), "-- lyrics helper\nlocal x = 1\n")
	writeStudioFile(t, filepath.Join(scripts, "ReaTeam Scripts", "Items", "Split at grid.eel"), "desc:Split items at grid\n// @version 1.0\n")
	writeStudioFile(t, filepath.Join(scripts, "Tools", "render.py"), "# @description Render stems\nimport reaper_python\n")
	writeStudioFile(t, filepath.Join(scripts, "ori-demo.lua"), "--[[ori\nname: Ori demo\ndescription: Created from the surface.\nconfirm: true\n]]--\nreturn 1\n")
	writeStudioFile(t, filepath.Join(scripts, "readme.txt"), "not a script")
	writeStudioFile(t, filepath.Join(library, "mine.lua"), "--[[ori\nname: Mine\ndescription: Library script.\nconfirm: false\n]]--\nreturn 1\n")
	kb := strings.Join([]string{
		"[Main]",
		`SCR 4 0 RS1ee9bb229dabffe151848d7efa3c10f748e1a1cf "Custom: lyrics.lua" Cockos/lyrics.lua`,
		`SCR 4 32060 RS7d3c_1ee9bb229dabffe151848d7efa3c10f748e1a1cf "Custom: lyrics.lua" Cockos/lyrics.lua`,
		`SCR 4 0 RS59bf9c1cf8a2bb77dd8133adf4398b84de85b840 "Custom: render.py" "` + filepath.Join(scripts, "Tools", "render.py") + `"`,
		`SCR 4 0 RS2222222222222222222222222222222222222222 "Custom: mine.lua" "` + filepath.Join(library, "mine.lua") + `"`,
		`SCR 4 0 RS3333333333333333333333333333333333333333 "Custom: elsewhere.lua" /Volumes/Other/elsewhere.lua`,
		`SCR 4 0 RS4444444444444444444444444444444444444444 "Custom: gone.lua" /Users/nobody/gone.lua`,
		"KEY 1 65 40044 0",
	}, "\n") + "\n"
	writeStudioFile(t, filepath.Join(resource, "reaper-kb.ini"), kb)
	writeStudioFile(t, filepath.Join(resource, "reapack.ini"), "[general]\nversion=1.2.4\n[remotes]\nsize=1\nremote0=ReaTeam Scripts|https://example.invalid/index.xml|1|0\n")
	roots = func() *studioRoots {
		resourceRoot, err := os.OpenRoot(resource)
		if err != nil {
			t.Fatal(err)
		}
		libraryRoot, err := os.OpenRoot(library)
		if err != nil {
			t.Fatal(err)
		}
		return &studioRoots{
			supported: true, installed: true, version: "7.80.0_fixture", layout: StudioLayoutPerUser,
			resource: resourceRoot, resourceState: StudioStateReady, resourcePath: resource, resourceDisplay: "~/Library/Application Support/REAPER",
			identity: resource, dev: 1, ino: 2, hasIdentity: true,
			library: libraryRoot, libraryState: StudioStateReady, libraryPath: library, libraryDisplay: "~/Ori Scripts/reaper",
		}
	}
	return resource, roots
}

func studioFacts(entry StudioEntry) map[string]string {
	facts := map[string]string{}
	for _, fact := range entry.Facts {
		facts[fact.ID] = fact.Value
	}
	return facts
}

func TestStudioScriptFactsComeOnlyFromInspectedMetadata(t *testing.T) {
	_, roots := scriptsFixture(t)
	service := &Service{studio: studioProbeStub{build: roots}}
	installation := roots().InstallationID()
	result, err := service.StudioInventory(context.Background(), StudioInventoryInput{SchemaVersion: 1, InstallationID: installation, SourceID: "reaper_scripts", Limit: 256})
	if err != nil || result.State != StudioStateOK {
		t.Fatalf("inventory = %+v, %v", result, err)
	}
	want := []string{"Cockos/lyrics.lua", "ReaTeam Scripts/Items/Split at grid.eel", "Tools/render.py", "Vocals/lyrics.lua", "ori-demo.lua"}
	if got := studioIDs(result.Entries); !slices.Equal(got, want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	byID := map[string]StudioEntry{}
	for _, entry := range result.Entries {
		byID[entry.NativeID] = entry
	}
	cockos := byID["Cockos/lyrics.lua"]
	if facts := studioFacts(cockos); facts[StudioFactLanguage] != "Lua" || facts[StudioFactRegistration] != studioRegisteredYes || facts[StudioFactPackage] != "" ||
		cockos.Description == nil || cockos.Description.Text != "Shows the lyrics of the current item." || cockos.Description.Source != studioDescriptionSource || cockos.Name != "lyrics" {
		t.Fatalf("Cockos/lyrics.lua = %+v", cockos)
	}
	vocals := byID["Vocals/lyrics.lua"]
	if facts := studioFacts(vocals); facts[StudioFactRegistration] != studioRegisteredNo || vocals.Description != nil || len(vocals.Facts) != 2 {
		t.Fatalf("an untagged comment became a description or registration was guessed: %+v", vocals)
	}
	split := byID["ReaTeam Scripts/Items/Split at grid.eel"]
	if facts := studioFacts(split); facts[StudioFactLanguage] != "EEL" || facts[StudioFactPackage] != "ReaTeam Scripts" || facts[StudioFactRegistration] != studioRegisteredNo ||
		split.Description == nil || split.Description.Text != "Split items at grid" {
		t.Fatalf("Split at grid = %+v", split)
	}
	render := byID["Tools/render.py"]
	if facts := studioFacts(render); facts[StudioFactLanguage] != "Python" || facts[StudioFactRegistration] != studioRegisteredYes || render.Description == nil || render.Description.Text != "Render stems" {
		t.Fatalf("render.py = %+v", render)
	}
	demo := byID["ori-demo.lua"]
	if demo.Description == nil || demo.Description.Text != "Created from the surface." || studioFacts(demo)[StudioFactRegistration] != studioRegisteredNo {
		t.Fatalf("ori-demo.lua = %+v", demo)
	}
	// Two rows point outside both sources; they are counted once, on the
	// first page of REAPER's own Scripts source, and never followed.
	// Four folders, five scripts, readme.txt and the Items folder are visited.
	if !reflect.DeepEqual(result.Coverage.Skipped, []StudioSkip{{Reason: studioSkipExternalRegistration, Count: 2}}) || result.Coverage.Visited != 11 {
		t.Fatalf("coverage = %+v", result.Coverage)
	}
	for _, entry := range result.Entries {
		if entry.Facts == nil {
			t.Fatalf("facts must be an array: %+v", entry)
		}
	}

	// The second page of the same source repeats no external count.
	paged, err := service.StudioInventory(context.Background(), StudioInventoryInput{SchemaVersion: 1, InstallationID: installation, SourceID: "reaper_scripts", Limit: 2})
	if err != nil || paged.NextCursor == "" || len(paged.Coverage.Skipped) != 1 {
		t.Fatalf("first page = %+v, %v", paged, err)
	}
	second, err := service.StudioInventory(context.Background(), StudioInventoryInput{SchemaVersion: 1, InstallationID: installation, SourceID: "reaper_scripts", Cursor: paged.NextCursor, Limit: 2})
	if err != nil || len(second.Coverage.Skipped) != 0 || len(second.Entries) != 2 {
		t.Fatalf("second page = %+v, %v", second, err)
	}

	// The library source matches absolute rows against its own folder.
	library, err := service.StudioInventory(context.Background(), StudioInventoryInput{SchemaVersion: 1, InstallationID: installation, SourceID: "ori_scripts", Limit: 256})
	if err != nil || library.State != StudioStateOK || len(library.Entries) != 1 || studioFacts(library.Entries[0])[StudioFactRegistration] != studioRegisteredYes ||
		library.Entries[0].NativeID != "mine.lua" || len(library.Coverage.Skipped) != 0 {
		t.Fatalf("library = %+v, %v", library, err)
	}
}

func TestStudioInventoryRefusesWhatTheContractRefuses(t *testing.T) {
	resource, roots := scriptsFixture(t)
	service := &Service{studio: studioProbeStub{build: roots}}
	installation := roots().InstallationID()
	for name, input := range map[string]StudioInventoryInput{
		"schema":       {SchemaVersion: 2, InstallationID: installation, SourceID: "reaper_scripts", Limit: 1},
		"short id":     {SchemaVersion: 1, InstallationID: "inst_ab", SourceID: "reaper_scripts", Limit: 1},
		"no prefix":    {SchemaVersion: 1, InstallationID: "abcdefghijklmnop", SourceID: "reaper_scripts", Limit: 1},
		"limit zero":   {SchemaVersion: 1, InstallationID: installation, SourceID: "reaper_scripts", Limit: 0},
		"limit high":   {SchemaVersion: 1, InstallationID: installation, SourceID: "reaper_scripts", Limit: 257},
		"long cursor":  {SchemaVersion: 1, InstallationID: installation, SourceID: "reaper_scripts", Cursor: strings.Repeat("a", 513), Limit: 1},
		"empty source": {SchemaVersion: 1, InstallationID: installation, SourceID: "", Limit: 1},
	} {
		if _, err := service.StudioInventory(context.Background(), input); err == nil {
			t.Errorf("%s: request was accepted", name)
		}
	}
	read := func(input StudioInventoryInput) StudioInventoryResult {
		result, err := service.StudioInventory(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		if result.Entries == nil || result.Coverage.Skipped == nil {
			t.Fatalf("arrays must be present: %+v", result)
		}
		return result
	}
	if got := read(StudioInventoryInput{SchemaVersion: 1, InstallationID: "inst_0000000000000000", SourceID: "reaper_scripts", Limit: 1}); got.State != StudioStateInstallationChanged || len(got.Entries) != 0 || got.NextCursor != "" {
		t.Fatalf("other installation = %+v", got)
	}
	if got := read(StudioInventoryInput{SchemaVersion: 1, InstallationID: installation, SourceID: "presets", Limit: 1}); got.State != StudioStateUnsupported {
		t.Fatalf("unknown source = %+v", got)
	}
	if got := read(StudioInventoryInput{SchemaVersion: 1, InstallationID: installation, SourceID: "reaper_scripts", Cursor: "at-3", Limit: 1}); got.State != StudioStateCursorReset {
		t.Fatalf("foreign cursor = %+v", got)
	}
	if got := read(StudioInventoryInput{SchemaVersion: 1, InstallationID: installation, SourceID: "track_templates", Limit: 1}); got.State != StudioStateAbsent {
		t.Fatalf("absent folder = %+v", got)
	}
	if _, err := os.Lstat(filepath.Join(resource, "TrackTemplates")); !os.IsNotExist(err) {
		t.Fatal("reading an absent source created its folder")
	}
	// Every entry a page returns fits the declared bounds, and a page never
	// exceeds the declared output size.
	page := read(StudioInventoryInput{SchemaVersion: 1, InstallationID: installation, SourceID: "reaper_scripts", Limit: 256})
	encoded, err := json.Marshal(page)
	if err != nil || len(encoded) > maxStudioInventoryOutputBytes {
		t.Fatalf("page is %d bytes, %v", len(encoded), err)
	}
	for _, entry := range page.Entries {
		if len(entry.NativeID) > maxStudioNativeIDBytes || strings.HasPrefix(entry.NativeID, "/") || strings.Contains(entry.NativeID, "..") || !profileLine(entry.Name, maxStudioNameChars) {
			t.Fatalf("entry out of bounds: %+v", entry)
		}
	}
}

func TestStudioSourcesReportPresenceWithoutReading(t *testing.T) {
	resource, roots := scriptsFixture(t)
	writeStudioFile(t, filepath.Join(resource, "ProjectTemplates", "Default.RPP"), "<REAPER_PROJECT\n>\n")
	if err := os.Symlink(filepath.Join(resource, "ProjectTemplates"), filepath.Join(resource, "TrackTemplates")); err != nil {
		t.Fatal(err)
	}
	probe := clientProbe{web: WebRemoteObservation{State: ProbeMissing}}
	service := &Service{studio: studioProbeStub{build: roots}, probes: ProbeSet{WebRemote: probe, Transport: probe, Runner: runnerProbeStub{observation: RunnerObservation{State: ProbeMissing}}}}
	if _, err := service.StudioSources(context.Background(), StudioSourcesInput{SchemaVersion: 2}); err == nil {
		t.Fatal("another schema version was accepted")
	}
	result, err := service.StudioSources(context.Background(), StudioSourcesInput{SchemaVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != 1 || !strings.HasPrefix(result.Installation.ID, "inst_") || len(result.Installation.ID) != 37 ||
		result.Installation.Label != "REAPER 7.80" || result.Installation.Layout != StudioLayoutPerUser || !result.Installation.AppInstalled || result.Installation.AppVersion != "7.80" {
		t.Fatalf("installation = %+v", result.Installation)
	}
	states := map[string]StudioSource{}
	for _, source := range result.Sources {
		states[source.ID] = source
	}
	if len(states) != 5 || states["project_templates"].State != StudioStateReady || states["track_templates"].State != StudioStateUnsupported ||
		states["reaper_scripts"].State != StudioStateReady || states["ori_scripts"].State != StudioStateReady || states["audio_plugins"].State != StudioStateWebRemoteOff {
		t.Fatalf("sources = %+v", result.Sources)
	}
	if states["track_templates"].Detail == "" || states["audio_plugins"].Detail == "" || states["project_templates"].Detail != "" {
		t.Fatalf("details = %+v", result.Sources)
	}
	if states["project_templates"].Location != "~/Library/Application Support/REAPER/ProjectTemplates" || states["ori_scripts"].Location != "~/Ori Scripts/reaper" ||
		states["audio_plugins"].Location != "REAPER (running)" || states["audio_plugins"].Access != StudioAccessLive || states["reaper_scripts"].Access != StudioAccessFile {
		t.Fatalf("locations = %+v", result.Sources)
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > maxStudioSourcesOutputBytes {
		t.Fatalf("sources answer is %d bytes, %v", len(encoded), err)
	}
	// The same folders give the same installation id; another folder does not.
	again, _ := service.StudioSources(context.Background(), StudioSourcesInput{SchemaVersion: 1})
	if again.Installation.ID != result.Installation.ID {
		t.Fatal("installation id is not stable")
	}
	other := roots()
	other.ino = 3
	if other.InstallationID() == result.Installation.ID {
		t.Fatal("another folder identity has the same installation id")
	}
	other.Close()
}

// The live source's state ladder: platform, installation, Web Remote, a
// running REAPER, a registered runner, a runner that carries the plugin-list
// read, in that order.
func TestStudioLiveSourceStateLadder(t *testing.T) {
	_, roots := scriptsFixture(t)
	for _, test := range []struct {
		name    string
		web     WebRemoteObservation
		live    LiveTransportObservation
		runner  RunnerObservation
		install bool
		want    string
	}{
		{"not installed", WebRemoteObservation{State: ProbeReady, Port: 2307}, LiveTransportObservation{State: TransportAvailable, Port: 2307}, RunnerObservation{State: ProbeReady, Version: 2}, false, StudioStateAbsent},
		{"web remote off", WebRemoteObservation{State: ProbeMissing}, LiveTransportObservation{}, RunnerObservation{}, true, StudioStateWebRemoteOff},
		{"web remote unreadable", WebRemoteObservation{State: ProbeUnknown}, LiveTransportObservation{}, RunnerObservation{}, true, StudioStateWebRemoteOff},
		{"closed", WebRemoteObservation{State: ProbeReady, Port: 2307}, LiveTransportObservation{State: TransportOffline}, RunnerObservation{}, true, StudioStateAppClosed},
		{"runner missing", WebRemoteObservation{State: ProbeReady, Port: 2307}, LiveTransportObservation{State: TransportAvailable, Port: 2307}, RunnerObservation{State: ProbeMissing}, true, StudioStateReaderMissing},
		{"runner invalid", WebRemoteObservation{State: ProbeReady, Port: 2307}, LiveTransportObservation{State: TransportAvailable, Port: 2307}, RunnerObservation{State: ProbeInvalid}, true, StudioStateReaderMissing},
		{"runner 0.10.1", WebRemoteObservation{State: ProbeReady, Port: 2307}, LiveTransportObservation{State: TransportAvailable, Port: 2307}, RunnerObservation{State: ProbeReady, CommandID: "_RS1", Version: 0}, true, StudioStateReaderOutdated},
		{"runner current", WebRemoteObservation{State: ProbeReady, Port: 2307}, LiveTransportObservation{State: TransportAvailable, Port: 2307}, RunnerObservation{State: ProbeReady, CommandID: "_RS1", Version: 2}, true, StudioStateReady},
	} {
		t.Run(test.name, func(t *testing.T) {
			probe := clientProbe{web: test.web, transport: test.live}
			build := func() *studioRoots {
				view := roots()
				view.installed = test.install
				return view
			}
			service := &Service{studio: studioProbeStub{build: build}, probes: ProbeSet{WebRemote: probe, Transport: probe, Runner: runnerProbeStub{observation: test.runner}}}
			result, err := service.StudioSources(context.Background(), StudioSourcesInput{SchemaVersion: 1})
			if err != nil {
				t.Fatal(err)
			}
			if got := result.Sources[4]; got.ID != "audio_plugins" || got.State != test.want {
				t.Fatalf("audio_plugins = %+v, want %s", got, test.want)
			}
		})
	}
	unsupported := &Service{}
	result, err := unsupported.StudioSources(context.Background(), StudioSourcesInput{SchemaVersion: 1})
	if err != nil || result.Installation.AppInstalled || result.Installation.Label != "REAPER (not installed)" {
		t.Fatalf("no probe = %+v, %v", result, err)
	}
	for _, source := range result.Sources {
		if source.State != StudioStateUnsupported {
			t.Fatalf("source without a platform = %+v", source)
		}
	}
}

// The service cuts its answers to bounds compiled into it; the host checks
// them against the manifest. The two sets must agree.
func TestStudioBoundsAreTheOnesTheManifestDeclares(t *testing.T) {
	manifest := repoManifest(t)
	var services []struct {
		Operations []struct {
			ID             string          `json:"id"`
			TimeoutClass   string          `json:"timeout_class"`
			Policy         string          `json:"policy"`
			MaxOutputBytes int             `json:"max_output_bytes"`
			InputSchema    json.RawMessage `json:"input_schema"`
			OutputSchema   json.RawMessage `json:"output_schema"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(manifest["services"], &services); err != nil {
		t.Fatal(err)
	}
	type text struct {
		MinLength int      `json:"minLength"`
		MaxLength int      `json:"maxLength"`
		Enum      []string `json:"enum"`
	}
	found := 0
	for _, operation := range services[0].Operations {
		switch operation.ID {
		case "studio.sources.read":
			found++
			if operation.MaxOutputBytes != maxStudioSourcesOutputBytes || operation.TimeoutClass != "fast" || operation.Policy != "read_only" {
				t.Errorf("studio.sources.read = %+v", operation)
			}
			var schema struct {
				Properties struct {
					Sources struct {
						Items struct {
							Properties struct {
								Detail text `json:"detail"`
								State  text `json:"state"`
							} `json:"properties"`
						} `json:"items"`
					} `json:"sources"`
					Installation struct {
						Properties struct {
							ID text `json:"id"`
						} `json:"properties"`
					} `json:"installation"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(operation.OutputSchema, &schema); err != nil {
				t.Fatal(err)
			}
			if schema.Properties.Sources.Items.Properties.Detail.MaxLength != maxStudioDetailChars ||
				schema.Properties.Installation.Properties.ID.MinLength != minStudioInstallationIDChars || schema.Properties.Installation.Properties.ID.MaxLength != maxStudioInstallationIDChars {
				t.Errorf("sources bounds differ: %+v", schema)
			}
			for _, state := range []string{StudioStateReady, StudioStateAbsent, StudioStateDenied, StudioStateUnsupported, StudioStateAppClosed, StudioStateReaderMissing, StudioStateReaderOutdated, StudioStateWebRemoteOff} {
				if !slices.Contains(schema.Properties.Sources.Items.Properties.State.Enum, state) {
					t.Errorf("source state %q is not declared", state)
				}
			}
		case "studio.inventory.read":
			found++
			if operation.MaxOutputBytes != maxStudioInventoryOutputBytes || operation.TimeoutClass != "normal" || operation.Policy != "read_only" {
				t.Errorf("studio.inventory.read = %+v", operation)
			}
			var input struct {
				Properties struct {
					Cursor text `json:"cursor"`
					Limit  struct {
						Minimum int `json:"minimum"`
						Maximum int `json:"maximum"`
					} `json:"limit"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(operation.InputSchema, &input); err != nil {
				t.Fatal(err)
			}
			if input.Properties.Cursor.MaxLength != maxStudioCursorBytes || input.Properties.Limit.Maximum != maxStudioPageEntries || input.Properties.Limit.Minimum != 1 {
				t.Errorf("inventory input bounds differ: %+v", input)
			}
			var output struct {
				Properties struct {
					State   text `json:"state"`
					Entries struct {
						MaxItems int `json:"maxItems"`
						Items    struct {
							Properties struct {
								Native text `json:"native_id"`
								Name   text `json:"name"`
								Facts  struct {
									MaxItems int `json:"maxItems"`
									Items    struct {
										Properties struct {
											Value text `json:"value"`
										} `json:"properties"`
									} `json:"items"`
								} `json:"facts"`
								Description struct {
									Properties struct {
										Text text `json:"text"`
									} `json:"properties"`
								} `json:"description"`
							} `json:"properties"`
						} `json:"items"`
					} `json:"entries"`
					NextCursor text `json:"next_cursor"`
					Coverage   struct {
						Properties struct {
							Skipped struct {
								Items struct {
									Properties struct {
										Reason text `json:"reason"`
									} `json:"properties"`
								} `json:"items"`
							} `json:"skipped"`
						} `json:"properties"`
					} `json:"coverage"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(operation.OutputSchema, &output); err != nil {
				t.Fatal(err)
			}
			item := output.Properties.Entries.Items.Properties
			if output.Properties.Entries.MaxItems != maxStudioPageEntries || item.Native.MaxLength != maxStudioNativeIDBytes || item.Name.MaxLength != maxStudioNameChars ||
				item.Facts.Items.Properties.Value.MaxLength != maxStudioFactValueChars || item.Description.Properties.Text.MaxLength != maxStudioDescriptionChars ||
				output.Properties.NextCursor.MaxLength != maxStudioCursorBytes {
				t.Errorf("inventory output bounds differ")
			}
			reasons := output.Properties.Coverage.Properties.Skipped.Items.Properties.Reason.Enum
			if !slices.Equal(reasons, []string{studioSkipSymlink, studioSkipOutsideSource, studioSkipUnreadable, studioSkipUnsupportedName, studioSkipTooLarge, studioSkipHidden, studioSkipExternalRegistration, studioSkipDepthLimit}) {
				t.Errorf("skip reasons = %v", reasons)
			}
			for _, state := range []string{StudioStateOK, StudioStateInstallationChanged, StudioStateCursorReset, StudioStateReaderOutdated} {
				if !slices.Contains(output.Properties.State.Enum, state) {
					t.Errorf("inventory state %q is not declared", state)
				}
			}
		}
	}
	if found != 2 {
		t.Fatalf("declared studio operations = %d", found)
	}
	if studioInventoryOutputReserve < 1024 || studioInventoryOutputReserve >= maxStudioInventoryOutputBytes/8 {
		t.Errorf("the reserve kept free of entries is %d of %d bytes", studioInventoryOutputReserve, maxStudioInventoryOutputBytes)
	}
}

// The inventory vector is what a page of this service looks like, and no
// invalid vector is a shape this service can produce.
func TestStudioInventoryVectorsDecodeAsThisServicesShapes(t *testing.T) {
	var valid StudioInventoryResult
	if err := decodeStrictTidyJSON(studioVector(t, "inventory.valid.json"), maxStudioInventoryOutputBytes, &valid); err != nil {
		t.Fatalf("inventory.valid.json: %v", err)
	}
	if valid.State != StudioStateOK || len(valid.Entries) != 3 || valid.NextCursor == "" {
		t.Fatalf("vector = %+v", valid)
	}
	if id, ok := decodeStudioCursor(valid.NextCursor); !ok || id != valid.Entries[2].NativeID {
		t.Fatalf("the vector's cursor %q is not this service's cursor for %q", valid.NextCursor, valid.Entries[2].NativeID)
	}
	var request StudioInventoryInput
	if err := decodeStrictTidyJSON(studioVector(t, "inventory.request.valid.json"), 4096, &request); err != nil || validateStudioInventoryInput(request) != nil {
		t.Fatalf("inventory.request.valid.json: %v", err)
	}
	var sources StudioSourcesResult
	if err := decodeStrictTidyJSON(studioVector(t, "sources.valid.json"), maxStudioSourcesOutputBytes, &sources); err != nil || len(sources.Sources) != 5 {
		t.Fatalf("sources.valid.json: %v", err)
	}
	for _, source := range sources.Sources {
		spec, ok := lookupStudioSource(source.ID)
		if !ok || spec.Category != source.Category || spec.Access != source.Access {
			t.Errorf("vector source %+v is not one this service declares", source)
		}
	}
}

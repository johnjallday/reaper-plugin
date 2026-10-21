package reaper

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestStudioFXInspectorPassesTheReadOnlyAuditAndNothingMore(t *testing.T) {
	if !strings.HasPrefix(canonicalStudioFXInspectorLua, readOnlyInspectorHeader) {
		t.Fatal("the plugin-list inspector does not select the reserved read-only mode")
	}
	if err := validateReadOnlyInspectorLua(canonicalStudioFXInspectorLua); err != nil {
		t.Fatalf("inspector audit: %v", err)
	}
	for _, forbidden := range []string{"reaper.Undo_", "reaper.Set", "reaper.Insert", "reaper.Delete", "reaper.Main_", "reaper.TrackFX_Add", "reaper.TrackFX_", "reaper.UpdateArrange", "reaper.defer"} {
		if strings.Contains(canonicalStudioFXInspectorLua, forbidden) {
			t.Fatalf("inspector contains %q", forbidden)
		}
	}
	mutated := canonicalStudioFXInspectorLua + "\nreaper.TrackFX_AddByName(reaper.GetTrack(0, 0), 'ReaEQ', false, 1)\n"
	if err := validateReadOnlyInspectorLua(mutated); !errors.Is(err, ErrReadOnlyInspectionRejected) {
		t.Fatalf("mutating inspector audit error = %v", err)
	}
	for _, required := range []string{"local MAX_OUTPUT_BYTES = 4 * 1024 * 1024", "local MAX_ROWS = 20000", "os.rename(temp_path, list_path)"} {
		if !strings.Contains(canonicalStudioFXInspectorLua, required) {
			t.Fatalf("inspector is missing %q", required)
		}
	}
}

// The installed Lua runner and the Go exchange carry the same allowlist and
// the same version number, so an old runner is told apart by the file it
// never wrote.
func TestRunnerLuaWritesItsVersionAndAllowsThePluginList(t *testing.T) {
	version := regexp.MustCompile(`(?m)^local runner_version = "(\d+)"$`).FindStringSubmatch(runnerLua)
	if len(version) != 2 || version[1] != strconv.Itoa(runnerVersion) {
		t.Fatalf("runner Lua version = %v, Go runnerVersion = %d", version, runnerVersion)
	}
	if !strings.Contains(runnerLua, `write_file(version_path, runner_version)`) || !strings.Contains(runnerLua, `"/runner.version"`) {
		t.Fatal("runner Lua does not write runner.version")
	}
	if !strings.Contains(runnerLua, "  EnumInstalledFX = true,") {
		t.Fatal("runner Lua does not allow EnumInstalledFX in read-only mode")
	}
	if _, ok := readOnlyAllowedAPIs["EnumInstalledFX"]; !ok {
		t.Fatal("Go audit does not allow EnumInstalledFX")
	}
	if _, ok := tidyApplierAllowedAPIs["EnumInstalledFX"]; ok {
		t.Fatal("the tidy applier gained the plugin-list read")
	}
	if studioReaderRunnerVersion > runnerVersion {
		t.Fatalf("the reader needs runner %d but the embedded runner is %d", studioReaderRunnerVersion, runnerVersion)
	}
}

func TestStudioFXEntryMapsTheObservedRows(t *testing.T) {
	for _, test := range []struct {
		row   studioFXRow
		id    string
		kind  string
		name  string
		facts map[string]string
	}{
		{studioFXRow{"VST3: ANIMATE (Mastering the Mix)", "/Library/Audio/Plug-Ins/VST3/ANIMATE.vst3"}, "VST3:ANIMATE.vst3", StudioKindEffect, "ANIMATE", map[string]string{"format": "VST3", "vendor": "Mastering the Mix"}},
		{studioFXRow{"VST3: Aphex Vintage Exciter Mono (Waves)", "/Library/Audio/Plug-Ins/VST3/WaveShell1-VST3 16.7.vst3<1189814795"}, "VST3:WaveShell1-VST3 16.7.vst3<1189814795", StudioKindEffect, "Aphex Vintage Exciter Mono", map[string]string{"format": "VST3", "vendor": "Waves"}},
		{studioFXRow{"VST3i: Battery 4 (Native Instruments) (64 out)", "/Library/Audio/Plug-Ins/VST3/Battery 4.vst3"}, "VST3:Battery 4.vst3", StudioKindInstrument, "Battery 4 (64 out)", map[string]string{"format": "VST3", "vendor": "Native Instruments"}},
		{studioFXRow{"VSTi: Battery 4 (x86_64) (Native Instruments GmbH) (32 out)", "/Library/Audio/Plug-Ins/VST/Battery 4.vst"}, "VST:Battery 4.vst", StudioKindInstrument, "Battery 4 (x86_64) (32 out)", map[string]string{"format": "VST", "vendor": "Native Instruments GmbH"}},
		{studioFXRow{"VST: Console Recall (Universal Audio) (mono)", "/Library/Audio/Plug-Ins/VST/Universal Audio/UAD Console/Console Recall.vst"}, "VST:Console Recall.vst", StudioKindEffect, "Console Recall (mono)", map[string]string{"format": "VST", "vendor": "Universal Audio"}},
		{studioFXRow{"AU: AUDelay (Apple)", "Apple: AUDelay"}, "AU:Apple: AUDelay", StudioKindEffect, "AUDelay", map[string]string{"format": "AU", "vendor": "Apple"}},
		{studioFXRow{"AUi: DLSMusicDevice (Apple)", "Apple: DLSMusicDevice"}, "AU:Apple: DLSMusicDevice", StudioKindInstrument, "DLSMusicDevice", map[string]string{"format": "AU", "vendor": "Apple"}},
		{studioFXRow{"CLAP: Pro-Q 4 (FabFilter)", "com.FabFilter.Pro-Q.4"}, "CLAP:com.FabFilter.Pro-Q.4", StudioKindEffect, "Pro-Q 4", map[string]string{"format": "CLAP", "vendor": "FabFilter"}},
		{studioFXRow{"JS: Pitch Shifter 2", "pitch/superpitch"}, "JS:pitch/superpitch", StudioKindEffect, "Pitch Shifter 2", map[string]string{"format": "JS"}},
		{studioFXRow{"JS: Gain Reduction Scope (Cockos)", "analysis/gain_reduction_scope"}, "JS:analysis/gain_reduction_scope", StudioKindEffect, "Gain Reduction Scope", map[string]string{"format": "JS", "vendor": "Cockos"}},
		{studioFXRow{"Video processor", "Video processor"}, "REAPER:Video processor", StudioKindEffect, "Video processor", map[string]string{}},
	} {
		t.Run(test.row.Name, func(t *testing.T) {
			entry, ok := studioFXEntry(test.row)
			if !ok {
				t.Fatal("row was refused")
			}
			if entry.NativeID != test.id || entry.Kind != test.kind || entry.Name != test.name || !reflect.DeepEqual(studioFacts(entry), test.facts) || entry.Facts == nil || entry.ModifiedAt != "" || entry.Description != nil {
				t.Fatalf("entry = %+v", entry)
			}
		})
	}
	for _, row := range []studioFXRow{
		{"", "x"}, {"VST3: A", ""}, {"VST3: A\nB (V)", "/a/A.vst3"}, {"VST3: Up (V)", "/Library/../etc.vst3"},
	} {
		if _, ok := studioFXEntry(row); ok {
			t.Errorf("row %+v was accepted", row)
		}
	}
}

func TestStudioFXPagesCountEveryRowOnce(t *testing.T) {
	rows := []studioFXRow{
		{"VST3: A (V)", "/Library/VST3/A.vst3"},
		{"VST3: A again (V)", "/Users/me/VST3/A.vst3"},
		{"AU: B (Apple)", "Apple: B"},
		{"bad\x01name", "bad"},
		{"JS: C", "misc/c"},
		{"VSTi: D (V) (2 out)", "/Library/VST/D.vst"},
	}
	whole := pageStudioFXList(rows, "", 256, 1<<20)
	if got := studioIDs(whole.entries); !reflect.DeepEqual(got, []string{"AU:Apple: B", "JS:misc/c", "VST3:A.vst3", "VST:D.vst"}) {
		t.Fatalf("entries = %v", got)
	}
	if whole.coverage.Visited != 6 || !reflect.DeepEqual(whole.coverage.Skipped, []StudioSkip{{Reason: studioSkipUnsupportedName, Count: 2}}) || whole.nextCursor != "" {
		t.Fatalf("coverage = %+v, cursor %q", whole.coverage, whole.nextCursor)
	}
	for _, limit := range []int{1, 2, 3} {
		var entries []StudioEntry
		visited, skips, cursor := 0, map[string]int{}, ""
		for pages := 0; ; pages++ {
			if pages > 8 {
				t.Fatal("paging does not end")
			}
			page := pageStudioFXList(rows, cursor, limit, 1<<20)
			entries = append(entries, page.entries...)
			visited += page.coverage.Visited
			for _, skip := range page.coverage.Skipped {
				skips[skip.Reason] += skip.Count
			}
			if page.nextCursor == "" {
				break
			}
			cursor, _ = decodeStudioCursor(page.nextCursor)
		}
		if !reflect.DeepEqual(entries, whole.entries) || visited != whole.coverage.Visited || !reflect.DeepEqual(studioSkipList(skips), whole.coverage.Skipped) {
			t.Fatalf("limit %d: entries %v visited %d skips %v", limit, studioIDs(entries), visited, skips)
		}
	}
}

func TestStudioFXListIsDecodedStrictly(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, studioFXListFileName)
	for name, body := range map[string]string{
		"count mismatch": `{"schema_version":1,"app_version":"7.80","fx":[{"name":"JS: A","ident":"a"}],"count":2}`,
		"unknown key":    `{"schema_version":1,"app_version":"7.80","fx":[],"count":0,"path":"/x"}`,
		"other schema":   `{"schema_version":2,"app_version":"7.80","fx":[],"count":0}`,
		"trailing data":  `{"schema_version":1,"app_version":"7.80","fx":[],"count":0} {}`,
	} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readStudioFXList(path); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"app_version":"7.80/macOS-arm64","fx":[{"name":"JS: A","ident":"a"}],"count":1}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := readStudioFXList(path)
	if err != nil || len(list.FX) != 1 || list.AppVersion != "7.80/macOS-arm64" {
		t.Fatalf("list = %+v, %v", list, err)
	}
}

// liveStudioFixture is a REAPER whose runner answers the plugin-list
// inspection: it checks the inbox carries the reserved header and the
// allowlisted enumeration, writes the list and reports ok. answer lets a test
// make it refuse the audit or stay silent instead.
func liveStudioFixture(t *testing.T, runnerVersion int, answer func(root string)) (*Service, string) {
	t.Helper()
	root := t.TempDir()
	commandID := "_RSstudio01"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/_/TRANSPORT":
			_, _ = w.Write([]byte("TRANSPORT\t0\t0\t0\t1.1.00\t1.1.00\n"))
		case "/_/" + commandID:
			inbox, err := os.ReadFile(filepath.Join(root, "inbox.lua"))
			if err != nil || !strings.HasPrefix(string(inbox), readOnlyInspectorHeader) || !strings.Contains(string(inbox), "reaper.EnumInstalledFX(") {
				t.Errorf("runner was triggered with %q, %v", inbox, err)
			}
			answer(root)
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(server.Close)
	port, err := strconv.Atoi(strings.TrimPrefix(server.URL, "http://127.0.0.1:"))
	if err != nil {
		t.Fatal(err)
	}
	probe := clientProbe{web: WebRemoteObservation{State: ProbeReady, Port: port}, transport: LiveTransportObservation{State: TransportAvailable, Port: port}}
	studio := studioProbeStub{build: func() *studioRoots {
		return &studioRoots{supported: true, installed: true, version: "7.80", layout: StudioLayoutPerUser, resourceState: StudioStateAbsent, libraryState: StudioStateAbsent, identity: "/fixture/REAPER"}
	}}
	probes := ProbeSet{
		WebRemote: probe, Transport: probe, Studio: studio,
		Runner: runnerProbeStub{observation: RunnerObservation{State: ProbeReady, Root: root, CommandID: commandID, Version: runnerVersion}},
	}
	service := NewService(&Manager{}, probes, runnerRootStub{root: root})
	service.client.http = server.Client()
	service.runner.timeout = 300 * time.Millisecond
	return service, studio.build().InstallationID()
}

func writeStudioFXAnswer(t *testing.T, body string) func(root string) {
	t.Helper()
	return func(root string) {
		if err := os.WriteFile(filepath.Join(root, studioFXListFileName), []byte(body), 0o600); err != nil {
			t.Error(err)
		}
		if err := os.WriteFile(filepath.Join(root, "last_status.txt"), []byte("ok\n"), 0o600); err != nil {
			t.Error(err)
		}
	}
}

func TestStudioLiveInventoryReadsThePluginListThroughTheAuditedRunner(t *testing.T) {
	list := `{"schema_version":1,"app_version":"7.80/macOS-arm64","fx":[` +
		`{"name":"VST3: ANIMATE (Mastering the Mix)","ident":"/Library/Audio/Plug-Ins/VST3/ANIMATE.vst3"},` +
		`{"name":"AUi: AUSampler (Apple)","ident":"Apple: AUSampler"},` +
		`{"name":"JS: Delay","ident":"delay/delay"}],"count":3}` + "\n"
	service, installation := liveStudioFixture(t, 2, writeStudioFXAnswer(t, list))
	result, err := service.StudioInventory(context.Background(), StudioInventoryInput{SchemaVersion: 1, InstallationID: installation, SourceID: "audio_plugins", Limit: 2})
	if err != nil || result.State != StudioStateOK {
		t.Fatalf("inventory = %+v, %v", result, err)
	}
	if got := studioIDs(result.Entries); !reflect.DeepEqual(got, []string{"AU:Apple: AUSampler", "JS:delay/delay"}) || result.NextCursor == "" || result.Coverage.Visited != 2 {
		t.Fatalf("first page = %+v", result)
	}
	if result.Entries[0].Kind != StudioKindInstrument || studioFacts(result.Entries[0])["vendor"] != "Apple" {
		t.Fatalf("entry = %+v", result.Entries[0])
	}
	second, err := service.StudioInventory(context.Background(), StudioInventoryInput{SchemaVersion: 1, InstallationID: installation, SourceID: "audio_plugins", Cursor: result.NextCursor, Limit: 2})
	if err != nil || second.State != StudioStateOK || len(second.Entries) != 1 || second.Entries[0].NativeID != "VST3:ANIMATE.vst3" || second.NextCursor != "" || second.Coverage.Visited != 1 {
		t.Fatalf("second page = %+v, %v", second, err)
	}
	sources, err := service.StudioSources(context.Background(), StudioSourcesInput{SchemaVersion: 1})
	if err != nil || sources.Sources[4].State != StudioStateReady {
		t.Fatalf("live source = %+v, %v", sources.Sources[4], err)
	}
}

func TestStudioLiveInventoryReportsAnOldOrSilentRunner(t *testing.T) {
	// An older runner is known before anything is triggered.
	old, installation := liveStudioFixture(t, 0, func(string) { t.Error("an outdated runner was triggered") })
	result, err := old.StudioInventory(context.Background(), StudioInventoryInput{SchemaVersion: 1, InstallationID: installation, SourceID: "audio_plugins", Limit: 256})
	if err != nil || result.State != StudioStateReaderOutdated || len(result.Entries) != 0 || result.NextCursor != "" {
		t.Fatalf("outdated = %+v, %v", result, err)
	}
	// A runner that claims the version but still refuses the API is outdated
	// too: the audit status is the evidence the spike recorded.
	refusing, installation := liveStudioFixture(t, 2, func(root string) {
		_ = os.WriteFile(filepath.Join(root, "last_status.txt"), []byte("error: read-only audit: API EnumInstalledFX is not read-only\n"), 0o600)
	})
	result, err = refusing.StudioInventory(context.Background(), StudioInventoryInput{SchemaVersion: 1, InstallationID: installation, SourceID: "audio_plugins", Limit: 256})
	if err != nil || result.State != StudioStateReaderOutdated {
		t.Fatalf("refusing = %+v, %v", result, err)
	}
	// A recorded runner that never answers is no reader (a stale runner.id).
	silent, installation := liveStudioFixture(t, 2, func(string) {})
	result, err = silent.StudioInventory(context.Background(), StudioInventoryInput{SchemaVersion: 1, InstallationID: installation, SourceID: "audio_plugins", Limit: 256})
	if err != nil || result.State != StudioStateReaderMissing {
		t.Fatalf("silent = %+v, %v", result, err)
	}
}

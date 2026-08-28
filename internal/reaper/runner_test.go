package reaper

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type runnerRootStub struct {
	root string
	err  error
}

func (s runnerRootStub) Resolve() (string, error) { return s.root, s.err }

type runnerProbeStub struct{ observation RunnerObservation }

func (s runnerProbeStub) DetectRunner(context.Context) RunnerObservation {
	return s.observation
}

func TestRunnerWritesInboxTriggersRegisteredRunnerAndReadsStatus(t *testing.T) {
	root := t.TempDir()
	commandID := "_RSdeadBEEF"
	actionCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/_/"+commandID {
			http.NotFound(w, req)
			return
		}
		actionCalls++
		if err := os.WriteFile(filepath.Join(root, "last_status.txt"), []byte("ok\n"), 0o600); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	port, err := strconv.Atoi(strings.TrimPrefix(server.URL, "http://127.0.0.1:"))
	if err != nil {
		t.Fatal(err)
	}
	probe := clientProbe{
		web:       WebRemoteObservation{State: ProbeReady, Port: port},
		transport: LiveTransportObservation{State: TransportAvailable, Port: port},
	}
	client := NewClient(ProbeSet{WebRemote: probe, Transport: probe})
	client.http = server.Client()
	runner := NewRunner(runnerRootStub{root: root}, ProbeSet{
		Runner: runnerProbeStub{observation: RunnerObservation{State: ProbeReady, Root: root, CommandID: commandID}},
	}, client)

	lua := "reaper.ShowConsoleMsg('hello')\n"
	result, err := runner.RunScript(context.Background(), lua)
	if err != nil || result.Outcome != "ok" || actionCalls != 1 {
		t.Fatalf("run = %+v, %v, calls=%d", result, err, actionCalls)
	}
	inbox, err := os.ReadFile(filepath.Join(root, "inbox.lua"))
	if err != nil || string(inbox) != lua {
		t.Fatalf("inbox = %q, %v", inbox, err)
	}
	if info, err := os.Stat(filepath.Join(root, "inbox.lua")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("inbox permissions = %+v, %v", info, err)
	}
}

func TestRunnerReadOnlyInspectionUsesOnlyReservedAuditedPath(t *testing.T) {
	root := t.TempDir()
	commandID := "_RSdeadBEEF"
	actionCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/_/"+commandID {
			http.NotFound(w, req)
			return
		}
		actionCalls++
		inbox, err := os.ReadFile(filepath.Join(root, "inbox.lua"))
		if err != nil {
			t.Error(err)
		} else if !strings.HasPrefix(string(inbox), readOnlyInspectorHeader) {
			t.Errorf("inbox did not carry reserved inspector header: %q", inbox)
		}
		if err := os.WriteFile(filepath.Join(root, "last_status.txt"), []byte("ok\n"), 0o600); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	port, err := strconv.Atoi(strings.TrimPrefix(server.URL, "http://127.0.0.1:"))
	if err != nil {
		t.Fatal(err)
	}
	probe := clientProbe{
		web:       WebRemoteObservation{State: ProbeReady, Port: port},
		transport: LiveTransportObservation{State: TransportAvailable, Port: port},
	}
	client := NewClient(ProbeSet{WebRemote: probe, Transport: probe})
	client.http = server.Client()
	runner := NewRunner(runnerRootStub{root: root}, ProbeSet{
		Runner: runnerProbeStub{observation: RunnerObservation{State: ProbeReady, Root: root, CommandID: commandID}},
	}, client)

	lua := readOnlyInspectorHeader + "local count = reaper.CountTracks(0)\n"
	result, err := runner.RunReadOnlyInspection(context.Background(), lua)
	if err != nil || result.Outcome != "ok" || actionCalls != 1 {
		t.Fatalf("read-only run = %+v, %v, calls=%d", result, err, actionCalls)
	}
	result, err = runner.RunScript(context.Background(), lua)
	if !errors.Is(err, ErrReadOnlyInspectionRejected) || result.Outcome != "error" || actionCalls != 1 {
		t.Fatalf("generic reserved-header run = %+v, %v, calls=%d", result, err, actionCalls)
	}
}

func TestReadOnlyInspectionAuditRejectsMutationAndIndirection(t *testing.T) {
	valid := readOnlyInspectorHeader + "local count = reaper.CountTracks(0)\n"
	if err := validateReadOnlyInspectorLua(valid); err != nil {
		t.Fatalf("valid inspector rejected: %v", err)
	}
	invalid := map[string]string{
		"missing header":            "local count = reaper.CountTracks(0)\n",
		"no REAPER read":            readOnlyInspectorHeader + "local count = 1\n",
		"mutation API":              readOnlyInspectorHeader + "reaper.SetMediaTrackInfo_Value(track, 'B_MUTE', 1)\n",
		"command API":               readOnlyInspectorHeader + "reaper.Main_OnCommand(40023, 0)\n",
		"dual read-write API":       readOnlyInspectorHeader + "reaper.GetSetMediaTrackInfo_String(track, 'P_NAME', 'changed', true)\n",
		"dynamic REAPER lookup":     readOnlyInspectorHeader + "reaper['CountTracks'](0)\n",
		"aliased REAPER table":      readOnlyInspectorHeader + "local api = reaper\napi.CountTracks(0)\n",
		"shell escape":              valid + "os.execute('curl example.invalid')\n",
		"runtime code loading":      valid + "load('return 1')()\n",
		"global environment access": valid + "local api = _G.reaper\n",
	}
	for name, script := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := validateReadOnlyInspectorLua(script); !errors.Is(err, ErrReadOnlyInspectionRejected) {
				t.Fatalf("audit error = %v", err)
			}
		})
	}
}

func TestInstalledRunnerKeepsInspectorOutsideUndoAndRefreshPath(t *testing.T) {
	deferStart := strings.Index(runnerLua, "if read_only then\n  -- A non-deferred ReaScript")
	deferEnd := strings.Index(runnerLua, "elseif tidy_applier then")
	start := strings.Index(runnerLua, "-- Deliberately no undo calls")
	applierStart := strings.Index(runnerLua, "-- Canonical tidy applier:")
	ordinaryStart := strings.Index(runnerLua, "-- Historical behavior for ordinary scripts:")
	if deferStart < 0 || deferEnd <= deferStart || start <= deferEnd || applierStart <= start || ordinaryStart <= applierStart {
		t.Fatalf("runner read-only and mutation branches are not explicit")
	}
	if strings.Count(runnerLua[deferStart:deferEnd], "reaper.defer(function() end)") != 1 {
		t.Fatalf("read-only runner must suppress REAPER's automatic action undo exactly once")
	}
	readOnlyBranch := runnerLua[start:applierStart]
	for _, forbidden := range []string{"Undo_BeginBlock", "Undo_EndBlock", "TrackList_AdjustWindows", "UpdateArrange"} {
		if strings.Contains(readOnlyBranch, forbidden) {
			t.Fatalf("read-only runner branch contains %s", forbidden)
		}
	}
	mutationBranch := runnerLua[ordinaryStart:]
	for _, required := range []string{"Undo_BeginBlock", "Undo_EndBlock", "TrackList_AdjustWindows", "UpdateArrange"} {
		if !strings.Contains(mutationBranch, required) {
			t.Fatalf("ordinary mutation runner lost %s", required)
		}
	}
	for api := range readOnlyAllowedAPIs {
		if !strings.Contains(runnerLua, api+" = true") {
			t.Fatalf("installed runner allowlist is missing %s", api)
		}
	}
}

func TestRunnerSurfacesREAPERErrorText(t *testing.T) {
	root := t.TempDir()
	commandID := "_RSdeadBEEF"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = os.WriteFile(filepath.Join(root, "last_status.txt"), []byte("error: attempt to call nil value\n"), 0o600)
	}))
	defer server.Close()
	port, _ := strconv.Atoi(strings.TrimPrefix(server.URL, "http://127.0.0.1:"))
	probe := clientProbe{
		web:       WebRemoteObservation{State: ProbeReady, Port: port},
		transport: LiveTransportObservation{State: TransportAvailable, Port: port},
	}
	client := NewClient(ProbeSet{WebRemote: probe, Transport: probe})
	client.http = server.Client()
	runner := NewRunner(runnerRootStub{root: root}, ProbeSet{
		Runner: runnerProbeStub{observation: RunnerObservation{State: ProbeReady, Root: root, CommandID: commandID}},
	}, client)
	result, err := runner.RunScript(context.Background(), "error('test')")
	if !errors.Is(err, ErrRunnerFailed) || result.Outcome != "error" || result.ErrorText != "attempt to call nil value" {
		t.Fatalf("runner error = %+v, %v", result, err)
	}
}

func TestRunnerDoesNotTouchInboxWhileDisconnected(t *testing.T) {
	root := t.TempDir()
	probe := clientProbe{
		web:       WebRemoteObservation{State: ProbeReady, Port: 2307},
		transport: LiveTransportObservation{State: TransportOffline},
	}
	client := NewClient(ProbeSet{WebRemote: probe, Transport: probe})
	runner := NewRunner(runnerRootStub{root: root}, ProbeSet{
		Runner: runnerProbeStub{observation: RunnerObservation{State: ProbeReady, Root: root, CommandID: "_RSdeadBEEF"}},
	}, client)
	runner.timeout = 50 * time.Millisecond
	result, err := runner.RunScript(context.Background(), "return 1")
	if !errors.Is(err, ErrActionDisconnected) || result.Outcome != "error" {
		t.Fatalf("disconnected run = %+v, %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(root, "inbox.lua")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disconnected run touched inbox: %v", err)
	}
}

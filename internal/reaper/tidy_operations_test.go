package reaper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRunTidySurveyUsesCanonicalInspectorAndPersistsProposal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	runnerRoot := filepath.Join(home, ".ori-reaper")
	if err := os.MkdirAll(runnerRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	projectRoot := t.TempDir()
	projectEntry := filepath.Join(projectRoot, "song.rpp")
	projectBytes := []byte("<REAPER_PROJECT 0.1\n>\n")
	if err := os.WriteFile(projectEntry, projectBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, tidyConventionsFileName), DefaultTidyConventions(), 0o600); err != nil {
		t.Fatal(err)
	}

	state := messyTidySurveyState()
	state.Project.Name = filepath.Base(projectEntry)
	state.Project.Path = projectEntry
	state.Project.ProjectChangeCount = 73
	stateData, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}

	commandID := "_RSdeadBEEF"
	actionCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/_/"+commandID {
			http.NotFound(w, req)
			return
		}
		actionCalls++
		inbox, readErr := os.ReadFile(filepath.Join(runnerRoot, "inbox.lua"))
		if readErr != nil {
			t.Error(readErr)
		} else if string(inbox) != canonicalTidyInspectorLua {
			t.Error("service did not run the embedded canonical inspector")
		}
		if writeErr := os.WriteFile(filepath.Join(runnerRoot, "state.json"), append(stateData, '\n'), 0o600); writeErr != nil {
			t.Error(writeErr)
		}
		if writeErr := os.WriteFile(filepath.Join(runnerRoot, "last_status.txt"), []byte("ok\n"), 0o600); writeErr != nil {
			t.Error(writeErr)
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
	probes := ProbeSet{
		WebRemote: probe, Transport: probe,
		Runner: runnerProbeStub{observation: RunnerObservation{State: ProbeReady, Root: runnerRoot, CommandID: commandID}},
	}
	service := NewService(NewManagerFromEnv(), probes, runnerRootStub{root: runnerRoot})
	service.client.http = server.Client()

	result, err := service.RunTidySurvey(context.Background(), HostContext{
		WorkspaceID: "workspace-1", WorkspaceRoot: projectRoot, ProjectEntry: projectEntry,
	})
	if err != nil {
		t.Fatalf("RunTidySurvey() error = %v", err)
	}
	if result.Outcome != "proposal" || result.AlreadyTidy || result.ItemCount == 0 || result.ProjectChangeCount != 73 || !validTidyID(result.ProposalID) {
		t.Fatalf("result = %+v", result)
	}
	if actionCalls != 1 {
		t.Fatalf("inspector action calls = %d, want 1", actionCalls)
	}
	stored, err := NewTidyArtifactStore(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := stored.ReadProposal(result.ProposalID)
	if err != nil || proposal.Plan == nil || len(proposal.Record.Items) != result.ItemCount {
		t.Fatalf("stored proposal = %+v, %v", proposal, err)
	}
	gotProject, err := os.ReadFile(projectEntry)
	if err != nil || string(gotProject) != string(projectBytes) {
		t.Fatalf("project changed during survey: %q, %v", gotProject, err)
	}
}

func TestRunTidyApplySelectionStagesOnlyCheckedItemsAndPersistsReport(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	runnerRoot := filepath.Join(home, ".ori-reaper")
	if err := os.MkdirAll(runnerRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	projectRoot := t.TempDir()
	projectEntry := filepath.Join(projectRoot, "song.rpp")
	if err := os.WriteFile(projectEntry, []byte("<REAPER_PROJECT 0.1\n>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewTidyArtifactStore(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	plan := validTidyPlanFixture()
	plan.InspectedProject.Name = filepath.Base(projectEntry)
	plan.InspectedProject.Path = projectEntry
	created := time.Date(2026, 8, 29, 1, 0, 0, 0, time.UTC)
	if _, err := store.WriteProposal(tidyProposalFromPlan(plan), created); err != nil {
		t.Fatal(err)
	}
	state := messyTidySurveyState()
	state.Project.Name = filepath.Base(projectEntry)
	state.Project.Path = projectEntry
	state.Project.ProjectChangeCount = 80
	stateData, _ := json.Marshal(state)

	commandID := "_RSdeadBEEF"
	actionCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/_/"+commandID {
			http.NotFound(w, req)
			return
		}
		actionCalls++
		inbox, readErr := os.ReadFile(filepath.Join(runnerRoot, "inbox.lua"))
		if readErr != nil {
			t.Error(readErr)
			return
		}
		switch {
		case strings.HasPrefix(string(inbox), readOnlyInspectorHeader):
			if writeErr := os.WriteFile(filepath.Join(runnerRoot, "state.json"), append(stateData, '\n'), 0o600); writeErr != nil {
				t.Error(writeErr)
			}
		case strings.HasPrefix(string(inbox), tidyApplierHeader):
			planData, planErr := os.ReadFile(filepath.Join(runnerRoot, "plan.json"))
			if planErr != nil {
				t.Error(planErr)
				return
			}
			selected, decodeErr := DecodeTidyEditPlan(planData)
			if decodeErr != nil {
				t.Error(decodeErr)
				return
			}
			items := make([]TidyApplyResultItem, len(selected.Items))
			for index, item := range selected.Items {
				items[index] = TidyApplyResultItem{ID: item.ID, Verb: item.Verb, Status: TidyApplyStatusApplied}
			}
			resultData, _ := json.Marshal(TidyApplyResult{
				SchemaVersion: TidySchemaVersion, PlanID: selected.PlanID,
				ProjectChangeCountBefore: 80, ProjectChangeCountAfter: 81, Items: items,
			})
			if writeErr := os.WriteFile(filepath.Join(runnerRoot, "apply_result.json"), append(resultData, '\n'), 0o600); writeErr != nil {
				t.Error(writeErr)
			}
		default:
			t.Error("unexpected runner script")
		}
		if writeErr := os.WriteFile(filepath.Join(runnerRoot, "last_status.txt"), []byte("ok\n"), 0o600); writeErr != nil {
			t.Error(writeErr)
		}
	}))
	defer server.Close()
	port, err := strconv.Atoi(strings.TrimPrefix(server.URL, "http://127.0.0.1:"))
	if err != nil {
		t.Fatal(err)
	}
	probe := clientProbe{web: WebRemoteObservation{State: ProbeReady, Port: port}, transport: LiveTransportObservation{State: TransportAvailable, Port: port}}
	probes := ProbeSet{WebRemote: probe, Transport: probe, Runner: runnerProbeStub{observation: RunnerObservation{State: ProbeReady, Root: runnerRoot, CommandID: commandID}}}
	service := NewService(NewManagerFromEnv(), probes, runnerRootStub{root: runnerRoot})
	service.client.http = server.Client()

	selectedIDs := []string{plan.Items[0].ID, plan.Items[2].ID}
	result, err := service.RunTidyApplySelection(context.Background(), HostContext{
		WorkspaceID: "workspace-1", WorkspaceRoot: projectRoot, ProjectEntry: projectEntry,
	}, TidyApplySelectionInput{ProposalID: plan.PlanID, SelectedItemIDs: selectedIDs})
	if err != nil {
		t.Fatalf("RunTidyApplySelection() error = %v", err)
	}
	if actionCalls != 2 || result.Outcome != TidyReportOutcomeApplied || len(result.Items) != 2 || result.UndoSummary != TidyUndoSentence {
		t.Fatalf("apply result = %+v, calls=%d", result, actionCalls)
	}
	stagedData, err := os.ReadFile(filepath.Join(runnerRoot, "plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	staged, err := DecodeTidyEditPlan(stagedData)
	if err != nil || len(staged.Items) != 2 || staged.Items[0].ID != selectedIDs[0] || staged.Items[1].ID != selectedIDs[1] {
		t.Fatalf("staged plan = %+v, %v", staged, err)
	}
	report, err := store.ReadApplyReport(plan.PlanID)
	if err != nil || len(report.Record.Items) != 2 {
		t.Fatalf("stored report = %+v, %v", report, err)
	}
}

func TestEmbeddedTidyScriptsMatchSkillAssets(t *testing.T) {
	assets := map[string]string{
		"inspect_project.lua": canonicalTidyInspectorLua,
		"apply_tidy_plan.lua": canonicalTidyApplierLua,
	}
	for name, embedded := range assets {
		data, err := os.ReadFile(filepath.Join("..", "..", "skills", "reaper-project-tidy", "scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != embedded {
			t.Fatalf("embedded service script %s drifted from the canonical skill asset", name)
		}
	}
}

func TestTidySurveyRejectsUnsafeOrMismatchedState(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "state.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := removeStaleTidyRunnerFile(link); err == nil {
		t.Fatal("expected stale symlink refusal")
	}
	if _, err := readTidyInspection(link); err == nil {
		t.Fatal("expected state symlink refusal")
	}
}

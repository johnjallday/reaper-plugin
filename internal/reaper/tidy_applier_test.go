package reaper

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func canonicalTidyApplier(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "skills", "reaper-project-tidy", "scripts", "apply_tidy_plan.lua")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read canonical applier: %v", err)
	}
	return string(data)
}

func TestCanonicalTidyApplierPassesDedicatedRunnerAudit(t *testing.T) {
	script := canonicalTidyApplier(t)
	if !strings.HasPrefix(script, tidyApplierHeader) {
		t.Fatalf("canonical applier does not select the reserved mutation mode")
	}
	if err := validateTidyApplierLua(script); err != nil {
		t.Fatalf("canonical applier audit error = %v", err)
	}
	if err := validateReadOnlyInspectorLua(script); !errors.Is(err, ErrReadOnlyInspectionRejected) {
		t.Fatalf("applier entered read-only mode: %v", err)
	}
}

func TestCanonicalTidyApplierMutationSurfaceIsCosmeticOnly(t *testing.T) {
	script := canonicalTidyApplier(t)
	wantCalls := map[string]int{
		"reaper.SetMediaTrackInfo_Value": 1,
		"reaper.SetProjectMarker3":       1,
		"reaper.DeleteProjectMarker":     1,
	}
	for call, want := range wantCalls {
		if got := strings.Count(script, call); got != want {
			t.Fatalf("%s calls = %d, want %d", call, got, want)
		}
	}
	if !strings.Contains(script, `reaper.SetMediaTrackInfo_Value(track, "I_CUSTOMCOLOR", native)`) {
		t.Fatalf("track mutation is not fixed to I_CUSTOMCOLOR")
	}
	for _, forbidden := range []string{
		"B_MUTE", "I_SOLO", "I_RECARM", "D_VOL", "D_PAN", "SetCurrentBPM",
		"TrackFX_", "CreateTrackSend", "SetTrackSendInfo", "SetMediaItemInfo",
		"Main_SaveProject", "RENDER_",
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("canonical applier contains sound-affecting surface %q", forbidden)
		}
	}
}

func TestCanonicalTidyApplierAuditRejectsInjectedSoundMutation(t *testing.T) {
	script := canonicalTidyApplier(t)
	mutated := script + "\nreaper.SetCurrentBPM(0, 200, true)\n"
	if err := validateTidyApplierLua(mutated); err == nil {
		t.Fatalf("sound-affecting applier passed runner audit")
	}
}

func TestCanonicalTidyApplierLiveEvidenceBindsResultAndRestoresInOneUndo(t *testing.T) {
	root := filepath.Join("..", "..", "scripts", "spikes", "reaper-project-tidy")
	planData, err := os.ReadFile(filepath.Join(root, "observed-edit-plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := DecodeTidyEditPlan(planData)
	if err != nil {
		t.Fatalf("decode observed plan: %v", err)
	}
	resultData, err := os.ReadFile(filepath.Join(root, "observed-apply-result.json"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := DecodeTidyApplyResult(resultData)
	if err != nil {
		t.Fatalf("decode observed result: %v", err)
	}
	if err := result.ValidateAgainstPlan(plan); err != nil {
		t.Fatalf("bind observed result to plan: %v", err)
	}
	for _, item := range result.Items {
		if item.Status != TidyApplyStatusApplied {
			t.Fatalf("live item was not applied: %+v", item)
		}
	}

	applied := readObservedTidyState(t, root, "observed-applied-state.json")
	undone := readObservedTidyState(t, root, "observed-after-one-undo.json")
	if applied.Tracks[1].Color == nil || undone.Tracks[1].Color != nil {
		t.Fatalf("track color did not restore in one undo")
	}
	appliedMarkers := tidyMarkerMap(applied.Markers)
	undoneMarkers := tidyMarkerMap(undone.Markers)
	if appliedMarkers["marker:1"].Name != "Chorus" || undoneMarkers["marker:1"].Name != "chorus" ||
		appliedMarkers["region:202"].Name != "Chorus 2" || undoneMarkers["region:202"].Name != "chorus 2" {
		t.Fatalf("marker/region names did not restore in one undo")
	}
	if _, exists := appliedMarkers["marker:2"]; exists {
		t.Fatalf("duplicate marker still existed after apply")
	}
	if _, exists := undoneMarkers["marker:2"]; !exists {
		t.Fatalf("deleted marker did not restore in one undo")
	}
}

func readObservedTidyState(t *testing.T, root, filename string) TidyInspectedState {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filename))
	if err != nil {
		t.Fatal(err)
	}
	state, err := DecodeTidyInspectedState(data)
	if err != nil {
		t.Fatalf("decode %s: %v", filename, err)
	}
	return state
}

func tidyMarkerMap(markers []TidyInspectedMarker) map[string]TidyInspectedMarker {
	result := make(map[string]TidyInspectedMarker, len(markers))
	for _, marker := range markers {
		kind := "marker:"
		if marker.IsRegion {
			kind = "region:"
		}
		result[kind+fmt.Sprint(marker.ID)] = marker
	}
	return result
}

func TestInstalledRunnerValidatesAndPreparesBeforeTidyUndo(t *testing.T) {
	branchStart := strings.Index(runnerLua, "-- Canonical tidy applier:")
	branchEnd := strings.Index(runnerLua, "-- Historical behavior for ordinary scripts:")
	if branchStart < 0 || branchEnd <= branchStart {
		t.Fatalf("dedicated tidy applier branch is missing")
	}
	branch := runnerLua[branchStart:branchEnd]
	prepare := strings.Index(branch, "pcall(chunk)")
	undoBegin := strings.Index(branch, "reaper.Undo_BeginBlock()")
	apply := strings.Index(branch, "pcall(contract.apply)")
	undoEnd := strings.Index(branch, "reaper.Undo_EndBlock")
	finalize := strings.Index(branch, "contract.finalize")
	if prepare < 0 || undoBegin <= prepare || apply <= undoBegin || undoEnd <= apply || finalize <= undoEnd {
		t.Fatalf("tidy runner phase order is invalid")
	}
	if !strings.Contains(branch, "if contract.has_mutations then") ||
		!strings.Contains(branch, "reaper.defer(function() end)") {
		t.Fatalf("tidy runner does not suppress empty/refused undo points")
	}
}

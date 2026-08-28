package reaper

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tidyTestGUID = "{DB7F23BB-38B6-3844-8A87-22FD017EE05B}"

func tidyString(value string) *string         { return &value }
func tidyInt(value int) *int                  { return &value }
func tidyFloat(value float64) *float64        { return &value }
func tidyColor(red, green, blue int) *TidyRGB { return &TidyRGB{Red: red, Green: green, Blue: blue} }

func validTidyStateFixture() TidyInspectedState {
	return TidyInspectedState{
		SchemaVersion: TidySchemaVersion,
		Project: TidyInspectedProject{
			Name: "Messy Project", Path: "/tmp/messy-project.RPP", SaveDirty: true, ProjectChangeCount: 41,
		},
		Tracks: []TidyInspectedTrack{
			{GUID: tidyTestGUID, Index: 0, Name: "Vox 2", Color: nil, FolderDepth: 0, ItemCount: 3, FXCount: 2},
			{GUID: "{771C2680-B9FA-2B48-AD94-ADFFA0A2A90C}", Index: 1, Name: "DRUM BUS", Color: tidyColor(12, 34, 56), FolderDepth: 1, ItemCount: 0, FXCount: 1},
		},
		Markers: []TidyInspectedMarker{
			{EnumerationIndex: 0, ID: 303, PositionSeconds: 10, Name: "chorus", Color: nil},
			{EnumerationIndex: 1, ID: 303, IsRegion: true, PositionSeconds: 20, EndSeconds: tidyFloat(30), Name: "Verse 1", Color: tidyColor(1, 2, 3)},
		},
	}
}

func validTidyPlanFixture() TidyEditPlan {
	return TidyEditPlan{
		SchemaVersion: TidySchemaVersion,
		PlanID:        "tidy-plan-001",
		InspectedProject: TidyPlanProjectSnapshot{
			Name: "Messy Project", Path: "/tmp/messy-project.RPP", ProjectChangeCount: 41,
		},
		Items: []TidyPlanItem{
			{
				ID: "color-vocals", Verb: TidyVerbSetTrackColor,
				Target:  TidyPlanTarget{TrackGUID: tidyTestGUID},
				Payload: TidyPlanPayload{Color: tidyColor(30, 90, 220)},
				Reason:  "conventions: vocals are blue",
			},
			{
				ID: "rename-marker-1", Verb: TidyVerbRenameMarker,
				Target:  TidyPlanTarget{MarkerID: tidyInt(1), SnapshotName: tidyString("chorus")},
				Payload: TidyPlanPayload{NewName: tidyString("Chorus")},
				Reason:  "conventions: markers use Title Case",
			},
			{
				ID: "rename-region-2", Verb: TidyVerbRenameRegion,
				Target:  TidyPlanTarget{MarkerID: tidyInt(2), SnapshotName: tidyString("verse 1")},
				Payload: TidyPlanPayload{NewName: tidyString("Verse 1")},
				Reason:  "conventions: regions use Title Case",
			},
			{
				ID: "delete-marker-3", Verb: TidyVerbDeleteMarker,
				Target:  TidyPlanTarget{MarkerID: tidyInt(3), SnapshotName: tidyString(""), SnapshotPositionSeconds: tidyFloat(10)},
				Payload: TidyPlanPayload{SurvivorMarkerID: tidyInt(1)},
				Reason:  "duplicate: empty marker at the exact same position as marker 1",
			},
		},
	}
}

func validTidyResultFixture() TidyApplyResult {
	return TidyApplyResult{
		SchemaVersion:            TidySchemaVersion,
		PlanID:                   "tidy-plan-001",
		ProjectChangeCountBefore: 44,
		ProjectChangeCountAfter:  47,
		Items: []TidyApplyResultItem{
			{ID: "color-vocals", Verb: TidyVerbSetTrackColor, Status: TidyApplyStatusApplied},
			{ID: "rename-marker-1", Verb: TidyVerbRenameMarker, Status: TidyApplyStatusApplied},
			{ID: "rename-region-2", Verb: TidyVerbRenameRegion, Status: TidyApplyStatusSkipped, Reason: "snapshot name changed"},
			{ID: "delete-marker-3", Verb: TidyVerbDeleteMarker, Status: TidyApplyStatusFailed, Error: "REAPER rejected marker deletion"},
		},
	}
}

func TestTidyInspectedStateContractAcceptsBoundedStableIdentities(t *testing.T) {
	fixture := validTidyStateFixture()
	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeTidyInspectedState(data)
	if err != nil {
		t.Fatalf("DecodeTidyInspectedState() error = %v", err)
	}
	if decoded.Project.ProjectChangeCount != 41 || decoded.Tracks[0].GUID != tidyTestGUID {
		t.Fatalf("decoded state = %+v", decoded)
	}
	if decoded.Markers[0].ID != decoded.Markers[1].ID || decoded.Markers[0].IsRegion == decoded.Markers[1].IsRegion {
		t.Fatalf("marker/region identity tuple was not preserved: %+v", decoded.Markers)
	}
}

func TestTidyInspectedStateRejectsMalformedShapes(t *testing.T) {
	tests := map[string]func(*TidyInspectedState){
		"schema version":            func(state *TidyInspectedState) { state.SchemaVersion = 2 },
		"noncontiguous track index": func(state *TidyInspectedState) { state.Tracks[1].Index = 9 },
		"duplicate guid":            func(state *TidyInspectedState) { state.Tracks[1].GUID = state.Tracks[0].GUID },
		"invalid color":             func(state *TidyInspectedState) { state.Tracks[0].Color = tidyColor(256, 0, 0) },
		"marker with end":           func(state *TidyInspectedState) { state.Markers[0].EndSeconds = tidyFloat(11) },
		"region without end":        func(state *TidyInspectedState) { state.Markers[1].EndSeconds = nil },
		"duplicate typed marker id": func(state *TidyInspectedState) {
			state.Markers = append(state.Markers, TidyInspectedMarker{EnumerationIndex: 2, ID: 303, PositionSeconds: 40})
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := validTidyStateFixture()
			mutate(&fixture)
			if err := fixture.Validate(); !errors.Is(err, ErrInvalidTidyState) {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestTidyPlanContractAllowsExactlyFourCosmeticVerbs(t *testing.T) {
	plan := validTidyPlanFixture()
	if err := plan.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	got := make(map[string]bool)
	for _, item := range plan.Items {
		got[item.Verb] = true
	}
	want := []string{TidyVerbSetTrackColor, TidyVerbRenameMarker, TidyVerbRenameRegion, TidyVerbDeleteMarker}
	for _, verb := range want {
		if !got[verb] || !validTidyVerb(verb) {
			t.Fatalf("verb %q is not in the frozen contract", verb)
		}
	}
	for _, forbidden := range []string{"rename_track", "move_item", "set_volume", "set_pan", "set_tempo", "set_fx", "set_routing", "set_render"} {
		if validTidyVerb(forbidden) {
			t.Fatalf("sound-affecting verb %q entered the contract", forbidden)
		}
	}
}

func TestDecodeTidyPlanRejectsUnknownVerbsAndMalformedPayloadsAsWholePlan(t *testing.T) {
	valid, err := json.Marshal(validTidyPlanFixture())
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string][]byte{
		"unknown verb":                 bytesReplaceOnce(t, valid, `"set_track_color"`, `"set_track_volume"`),
		"unexpected payload field":     bytesReplaceOnce(t, valid, `"color":{"red":30`, `"volume_db":-3,"color":{"red":30`),
		"missing custom color":         bytesReplaceOnce(t, valid, `"color":{"red":30,"green":90,"blue":220}`, `"new_name":"Blue"`),
		"marker target for track verb": bytesReplaceOnce(t, valid, `"track_guid":"`+tidyTestGUID+`"`, `"marker_id":9,"snapshot_name":"Vox 2"`),
		"delete without survivor":      bytesReplaceOnce(t, valid, `"survivor_marker_id":1`, `"new_name":"gone"`),
		"duplicate item id":            bytesReplaceOnce(t, valid, `"rename-marker-1"`, `"color-vocals"`),
		"duplicate JSON key":           []byte(`{"schema_version":1,"schema_version":1}`),
		"trailing document":            append(append([]byte(nil), valid...), []byte(` {}`)...),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			decoded, err := DecodeTidyEditPlan(data)
			if !errors.Is(err, ErrInvalidTidyPlan) || len(decoded.Items) != 0 {
				t.Fatalf("DecodeTidyEditPlan() = %+v, %v", decoded, err)
			}
		})
	}
}

func TestTidyPlanRejectsAmbiguousOrUnboundedItems(t *testing.T) {
	tests := map[string]func(*TidyEditPlan){
		"missing reason": func(plan *TidyEditPlan) { plan.Items[0].Reason = "" },
		"bad guid":       func(plan *TidyEditPlan) { plan.Items[0].Target.TrackGUID = "track-1" },
		"rename no-op":   func(plan *TidyEditPlan) { plan.Items[1].Payload.NewName = tidyString("chorus") },
		"millisecond delete target": func(plan *TidyEditPlan) {
			plan.Items[3].Target.SnapshotPositionSeconds = tidyFloat(maxTidyPositionSeconds + 1)
		},
		"delete target is survivor":       func(plan *TidyEditPlan) { plan.Items[3].Payload.SurvivorMarkerID = tidyInt(3) },
		"same marker renamed and deleted": func(plan *TidyEditPlan) { plan.Items[3].Target.MarkerID = tidyInt(1) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			plan := validTidyPlanFixture()
			mutate(&plan)
			if err := plan.Validate(); !errors.Is(err, ErrInvalidTidyPlan) {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestTidyApplyResultContractBindsEveryRowToReviewedPlan(t *testing.T) {
	plan := validTidyPlanFixture()
	result := validTidyResultFixture()
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeTidyApplyResult(data)
	if err != nil {
		t.Fatalf("DecodeTidyApplyResult() error = %v", err)
	}
	if err := decoded.ValidateAgainstPlan(plan); err != nil {
		t.Fatalf("ValidateAgainstPlan() error = %v", err)
	}

	decoded.Items[0], decoded.Items[1] = decoded.Items[1], decoded.Items[0]
	if err := decoded.ValidateAgainstPlan(plan); !errors.Is(err, ErrInvalidTidyResult) {
		t.Fatalf("reordered result error = %v", err)
	}
}

func TestTidyApplyResultRejectsOutcomeShapeConfusion(t *testing.T) {
	tests := map[string]func(*TidyApplyResult){
		"count decreases":        func(result *TidyApplyResult) { result.ProjectChangeCountAfter = 1 },
		"applied with reason":    func(result *TidyApplyResult) { result.Items[0].Reason = "changed" },
		"skipped without reason": func(result *TidyApplyResult) { result.Items[2].Reason = "" },
		"failed without error":   func(result *TidyApplyResult) { result.Items[3].Error = "" },
		"unknown status":         func(result *TidyApplyResult) { result.Items[0].Status = "partial" },
		"unknown verb":           func(result *TidyApplyResult) { result.Items[0].Verb = "set_volume" },
		"duplicate id":           func(result *TidyApplyResult) { result.Items[1].ID = result.Items[0].ID },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			result := validTidyResultFixture()
			mutate(&result)
			if err := result.Validate(); !errors.Is(err, ErrInvalidTidyResult) {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestTidyDecodersEnforceDocumentBoundsAndClosedObjects(t *testing.T) {
	planJSON, err := json.Marshal(validTidyPlanFixture())
	if err != nil {
		t.Fatal(err)
	}
	unknown := bytesReplaceOnce(t, planJSON, `"schema_version":1`, `"schema_version":1,"extra":true`)
	if _, err := DecodeTidyEditPlan(unknown); !errors.Is(err, ErrInvalidTidyPlan) {
		t.Fatalf("unknown field error = %v", err)
	}
	oversized := []byte(`{"padding":"` + strings.Repeat("x", maxTidyPlanBytes) + `"}`)
	if _, err := DecodeTidyEditPlan(oversized); !errors.Is(err, ErrInvalidTidyPlan) {
		t.Fatalf("oversized error = %v", err)
	}
}

func TestCheckedInTidySchemasAndExamplesStayExecutable(t *testing.T) {
	schemaRoot := filepath.Join("..", "..", "skills", "reaper-project-tidy", "schema")
	for _, filename := range []string{"inspected-state-v1.json", "edit-plan-v1.json", "apply-result-v1.json"} {
		data, err := os.ReadFile(filepath.Join(schemaRoot, filename))
		if err != nil {
			t.Fatalf("read schema %s: %v", filename, err)
		}
		var document map[string]any
		if err := json.Unmarshal(data, &document); err != nil {
			t.Fatalf("parse schema %s: %v", filename, err)
		}
		if document["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
			t.Fatalf("schema %s has no frozen draft declaration", filename)
		}
	}

	examples := []struct {
		filename string
		decode   func([]byte) error
	}{
		{"inspected-state-v1.json", func(data []byte) error { _, err := DecodeTidyInspectedState(data); return err }},
		{"edit-plan-v1.json", func(data []byte) error { _, err := DecodeTidyEditPlan(data); return err }},
		{"apply-result-v1.json", func(data []byte) error { _, err := DecodeTidyApplyResult(data); return err }},
	}
	for _, example := range examples {
		data, err := os.ReadFile(filepath.Join(schemaRoot, "examples", example.filename))
		if err != nil {
			t.Fatalf("read example %s: %v", example.filename, err)
		}
		if err := example.decode(data); err != nil {
			t.Fatalf("validate example %s: %v", example.filename, err)
		}
	}
}

func bytesReplaceOnce(t *testing.T, input []byte, old, replacement string) []byte {
	t.Helper()
	if !strings.Contains(string(input), old) {
		t.Fatalf("fixture does not contain %q", old)
	}
	return []byte(strings.Replace(string(input), old, replacement, 1))
}

package reaper

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func readQuestDocument(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- callers supply fixed checked-in repository fixture paths
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func TestPluginOwnsProjectOnlySetupQuestV4(t *testing.T) {
	manifest := readQuestDocument(t, "../../.ori-plugin/plugin.json")
	var features []string
	if err := json.Unmarshal(manifest["requires_host_features"], &features); err != nil {
		t.Fatal(err)
	}
	// setup_quests_v2 is the four-step host contract. Quest declaration v3
	// changed only the project-team staffing copy after Home ownership moved to
	// Music Project Management, and v4 changes that copy again for the single
	// REAPER Assistant; a host that knows only setup_quests_v1 refuses it.
	if !slices.Contains(features, "setup_quests_v2") || slices.Contains(features, "setup_quests_v1") {
		t.Fatalf("quest feature must be exactly setup_quests_v2: %v", features)
	}
	var quests []map[string]json.RawMessage
	if err := json.Unmarshal(manifest["setup_quests"], &quests); err != nil || len(quests) != 1 {
		t.Fatalf("expected one plugin-owned quest: %v (%v)", quests, err)
	}
	// Ori's generated install quest owns installation, and the workspace Setup
	// Wizard owns live-control readiness. Derive v4 from the frozen v1 fixture so
	// every field except that prior v2 extraction and the staffing copy is pinned.
	baseline := readQuestDocument(t, "testdata/setup-quest-migration/quest-v1.json")
	assertQuestJSONEqual(t, quests[0], questV4FromV1(t, baseline))

	var shape struct {
		Steps []struct {
			ID          string `json:"id"`
			Kind        string `json:"kind"`
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"steps"`
		WorkspaceLaunch map[string]string `json:"workspace_launch"`
	}
	encoded, err := json.Marshal(quests[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &shape); err != nil {
		t.Fatal(err)
	}
	wantSteps := [][2]string{
		{"project", "project_connect"}, {"workspace", "workspace_setup"},
		{"staffing", "assistant_program_staffing"}, {"summary", "summary"},
	}
	if len(shape.Steps) != len(wantSteps) {
		t.Fatalf("steps = %+v, want the four project_setup steps", shape.Steps)
	}
	for index, step := range shape.Steps {
		if step.ID != wantSteps[index][0] || step.Kind != wantSteps[index][1] {
			t.Fatalf("step %d = %+v, want %v", index, step, wantSteps[index])
		}
	}
	staffing := shape.Steps[2]
	if staffing.Title != "Add this project's REAPER assistant" || !strings.Contains(staffing.Description, "REAPER Assistant") ||
		!strings.Contains(staffing.Description, "staffed separately by Music Project Management") ||
		strings.Contains(staffing.Description, "Songwriter") {
		t.Fatalf("project-only staffing copy = %+v", staffing)
	}
	if len(shape.WorkspaceLaunch) != 2 || shape.WorkspaceLaunch["group_title"] == "" || shape.WorkspaceLaunch["group_name"] == "" {
		t.Fatalf("workspace_launch must carry only group_title and group_name: %v", shape.WorkspaceLaunch)
	}

	var identity struct {
		ID                         string `json:"id"`
		Version                    int    `json:"version"`
		SchemaVersion              int    `json:"schema_version"`
		IntegrationKey             string `json:"integration_key"`
		ExpectedBlueprintID        string `json:"expected_blueprint_id"`
		ExpectedAssistantProgramID string `json:"expected_assistant_program_id"`
	}
	data, err := json.Marshal(quests[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &identity); err != nil {
		t.Fatal(err)
	}
	if identity.ID != "reaper_setup" || identity.Version != 4 || identity.SchemaVersion != 1 ||
		identity.IntegrationKey != "ori_reaper" || identity.ExpectedBlueprintID != "reaper-song" ||
		identity.ExpectedAssistantProgramID != "music-producer-assistant" {
		t.Fatalf("quest migration changed durable identity: %+v", identity)
	}
	if _, ok := quests[0]["workspace_launch"]; !ok {
		t.Fatal("pre-workspace setup must remain separate from the workspace wizard")
	}
}

func TestQuestExtractionAndGroupContractLeaveOtherTemplateFieldsUnchanged(t *testing.T) {
	template := readQuestDocument(t, "../../blueprints/reaper-song/template.json")
	var reference string
	if err := json.Unmarshal(template["setup_quest"], &reference); err != nil || reference != "reaper_setup" {
		t.Fatalf("template must reference its owner's exact quest: %q (%v)", reference, err)
	}
	delete(template, "setup_quest")
	delete(template, "group_requirement")
	delete(template, "standalone_composition")
	delete(template, "assistant_project")
	delete(template, "inputs")
	// Blueprint v4 from published plugin v0.5.0, source 1f494db5. The quest
	// reference, split group/project declarations, and v8 inputs are the later
	// top-level additions. The old combined assistant_program is deliberately
	// excluded because v9 transfers that authority to the independent package.
	// Wizard, file-only mode, permissions, project connection, and the
	// authoritative .rpp entry remain fixed. The first starter task's 0.7.0 prose
	// is reconciled only after proving that its details are the sole change.
	baseline := readQuestDocument(t, "testdata/setup-quest-migration/template-v4.json")
	delete(baseline, "assistant_program")
	baseline["starter_tasks"] = withRewordedFirstStarterTask(t, baseline["starter_tasks"], template["starter_tasks"])
	assertQuestJSONEqual(t, template, baseline)
}

// withRewordedFirstStarterTask copies the current first starter task's details
// onto the frozen baseline, after checking that its details are the only field
// that changed. Requirement 47 reworded that task because the session is now
// created with the tempo and time signature the user chose; everything else
// about the task — what it is, what it requires, when it runs — must be
// exactly what v4 froze, and this fails if any of it moved.
func withRewordedFirstStarterTask(t *testing.T, baselineRaw, currentRaw json.RawMessage) json.RawMessage {
	t.Helper()
	var baseline, current []map[string]any
	if err := json.Unmarshal(baselineRaw, &baseline); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(currentRaw, &current); err != nil {
		t.Fatal(err)
	}
	if len(baseline) == 0 || len(current) != len(baseline) {
		t.Fatalf("starter task count changed: baseline %d, current %d", len(baseline), len(current))
	}
	first, currentFirst := baseline[0], current[0]
	if len(first) != len(currentFirst) {
		t.Fatalf("the first starter task gained or lost a field: %v vs %v", first, currentFirst)
	}
	for key, want := range first {
		if key == "details" {
			continue
		}
		if fmt.Sprint(currentFirst[key]) != fmt.Sprint(want) {
			t.Fatalf("the reword changed %q: %v → %v", key, want, currentFirst[key])
		}
	}
	details, ok := currentFirst["details"].(string)
	if !ok || details == first["details"] {
		t.Fatal("the first starter task's details are unchanged; this adjustment is stale")
	}
	first["details"] = details
	data, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// questV4FromV1 derives the expected version 4 declaration from the frozen v1
// fixture: no integration_install step, launch copy reduced to group fields,
// and staffing copy limited to this project's one REAPER Assistant.
func questV4FromV1(t *testing.T, v1 map[string]json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	result := make(map[string]json.RawMessage, len(v1))
	for key, raw := range v1 {
		result[key] = raw
	}
	result["version"] = json.RawMessage(`4`)

	var steps []map[string]any
	if err := json.Unmarshal(v1["steps"], &steps); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 5 || steps[0]["kind"] != "integration_install" {
		t.Fatalf("v1 fixture is not the five-step declaration: %v", steps)
	}
	var err error
	steps = steps[1:]
	for _, step := range steps {
		if step["id"] == "staffing" {
			step["title"] = "Add this project's REAPER assistant"
			step["description"] = "Add the project-local REAPER Assistant, who handles everything REAPER-related for this project. Music Production Home roles are staffed separately by Music Project Management."
		}
	}
	if result["steps"], err = json.Marshal(steps); err != nil {
		t.Fatal(err)
	}

	var launch map[string]any
	if err := json.Unmarshal(v1["workspace_launch"], &launch); err != nil {
		t.Fatal(err)
	}
	if result["workspace_launch"], err = json.Marshal(map[string]any{
		"group_title": launch["group_title"], "group_name": launch["group_name"],
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

func assertQuestJSONEqual(t *testing.T, actual, expected map[string]json.RawMessage) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("migration changed document fields: got %d, want %d", len(actual), len(expected))
	}
	for key, raw := range expected {
		var want, got any
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(actual[key], &got); err != nil {
			t.Fatalf("missing or invalid %s: %v", key, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("migration changed %s: got %s, want %s", key, actual[key], raw)
		}
	}
}

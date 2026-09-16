package reaper

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
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

func TestPluginOwnsFourStepSetupQuestV2(t *testing.T) {
	manifest := readQuestDocument(t, "../../.ori-plugin/plugin.json")
	var features []string
	if err := json.Unmarshal(manifest["requires_host_features"], &features); err != nil {
		t.Fatal(err)
	}
	// setup_quests_v2 is the four-step contract. A host that knows only v1 must
	// refuse this manifest rather than misread it, and v1 is no longer declared.
	if !slices.Contains(features, "setup_quests_v2") || slices.Contains(features, "setup_quests_v1") {
		t.Fatalf("quest feature must be exactly setup_quests_v2: %v", features)
	}
	var quests []map[string]json.RawMessage
	if err := json.Unmarshal(manifest["setup_quests"], &quests); err != nil || len(quests) != 1 {
		t.Fatalf("expected one plugin-owned quest: %v (%v)", quests, err)
	}
	// Ori's generated install quest now owns installing the plugin, and the
	// workspace Setup Wizard owns live-control readiness. Version 2 is the v1
	// declaration (Ori PR #466, commit 1846c497) with exactly those two parts
	// removed; every other field, step ID and display string is unchanged.
	baseline := readQuestDocument(t, "testdata/setup-quest-migration/quest-v1.json")
	assertQuestJSONEqual(t, quests[0], questV2FromV1(t, baseline))

	var shape struct {
		Steps []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
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
	if identity.ID != "reaper_setup" || identity.Version != 2 || identity.SchemaVersion != 1 ||
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
	// Blueprint v4 from published plugin v0.5.0, source 1f494db5. The quest
	// reference and v6 group declarations are the only later top-level additions;
	// wizard, file-only mode, grouped scopes/prompts, permissions, project
	// connection, authoritative .rpp entry, and normal starter tasks stay fixed.
	// The one later removal is the retired role "type" key (0.6.1), so it is
	// dropped from the frozen baseline rather than rewriting the fixture.
	baseline := readQuestDocument(t, "testdata/setup-quest-migration/template-v4.json")
	baseline["assistant_program"] = withoutRetiredRoleType(t, baseline["assistant_program"])
	assertQuestJSONEqual(t, template, baseline)
}

// withoutRetiredRoleType removes the "type" key Ori retired
// (johnjallday/ori-agent#490) from every assistant_program role, and fails if
// the baseline carried none: the removal must be the only difference it hides.
func withoutRetiredRoleType(t *testing.T, raw json.RawMessage) json.RawMessage {
	t.Helper()
	var program map[string]any
	if err := json.Unmarshal(raw, &program); err != nil {
		t.Fatal(err)
	}
	roles, _ := program["roles"].([]any)
	removed := 0
	for _, entry := range roles {
		if role, ok := entry.(map[string]any); ok {
			if _, has := role["type"]; has {
				delete(role, "type")
				removed++
			}
		}
	}
	if removed == 0 {
		t.Fatal("baseline assistant_program roles carry no retired type key; this adjustment is stale")
	}
	data, err := json.Marshal(program)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// questV2FromV1 derives the expected version 2 declaration from the frozen v1
// fixture: version 2, no integration_install step, and launch copy reduced to
// the group fields.
func questV2FromV1(t *testing.T, v1 map[string]json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	result := make(map[string]json.RawMessage, len(v1))
	for key, raw := range v1 {
		result[key] = raw
	}
	result["version"] = json.RawMessage(`2`)

	var steps []map[string]any
	if err := json.Unmarshal(v1["steps"], &steps); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 5 || steps[0]["kind"] != "integration_install" {
		t.Fatalf("v1 fixture is not the five-step declaration: %v", steps)
	}
	var err error
	if result["steps"], err = json.Marshal(steps[1:]); err != nil {
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

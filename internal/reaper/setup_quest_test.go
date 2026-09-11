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

func TestPluginOwnsUnchangedSetupQuestV1(t *testing.T) {
	manifest := readQuestDocument(t, "../../.ori-plugin/plugin.json")
	var features []string
	if err := json.Unmarshal(manifest["requires_host_features"], &features); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(features, "setup_quests_v1") {
		t.Fatal("older hosts must refuse a quest-capable contribution")
	}
	var quests []map[string]json.RawMessage
	if err := json.Unmarshal(manifest["setup_quests"], &quests); err != nil || len(quests) != 1 {
		t.Fatalf("expected one plugin-owned quest: %v (%v)", quests, err)
	}
	// The frozen pre-extraction declaration from Ori PR #466, commit 1846c497.
	// Compare the entire parsed document, including unknown keys: this migration
	// must not add an executor, change step IDs/version, or revise display copy.
	baseline := readQuestDocument(t, "testdata/setup-quest-migration/quest-v1.json")
	assertQuestJSONEqual(t, quests[0], baseline)

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
	if identity.ID != "reaper_setup" || identity.Version != 1 || identity.SchemaVersion != 1 ||
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
	baseline := readQuestDocument(t, "testdata/setup-quest-migration/template-v4.json")
	assertQuestJSONEqual(t, template, baseline)
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

package reaper

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// The vectors under testdata/resource-catalog are copies of Ori's
// internal/resourcecatalog/testdata/contract. They are the wire contract both
// sides test against; a change there is a contract change Ori must mirror.
func studioVector(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "resource-catalog", name)) // #nosec G304 -- fixed test vectors
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func decodeAny(t *testing.T, data []byte) any {
	t.Helper()
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func repoManifest(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".ori-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

// The resource_catalog declaration is part of the plugin's trusted fingerprint
// (contract §3.4, §13): it must be the exact document Ori's vectors describe,
// and it must point at a blueprint of this contribution whose team is a
// resource team and at two read-only operations of the declared service.
func TestStudioResourceCatalogDeclarationIsTheContractVector(t *testing.T) {
	manifest := repoManifest(t)
	raw, ok := manifest["resource_catalog"]
	if !ok {
		t.Fatal("the manifest declares no resource_catalog")
	}
	if !reflect.DeepEqual(decodeAny(t, raw), decodeAny(t, studioVector(t, "declaration.valid.json"))) {
		t.Fatalf("resource_catalog differs from declaration.valid.json:\n%s", raw)
	}
	var invalid []struct {
		Name   string          `json:"name"`
		Answer json.RawMessage `json:"answer"`
		Raw    string          `json:"raw"`
	}
	if err := json.Unmarshal(studioVector(t, "declaration.invalid.json"), &invalid); err != nil {
		t.Fatal(err)
	}
	for _, vector := range invalid {
		if len(vector.Answer) > 0 && reflect.DeepEqual(decodeAny(t, raw), decodeAny(t, vector.Answer)) {
			t.Errorf("the declaration equals the invalid vector %q", vector.Name)
		}
	}

	var declaration struct {
		BlueprintID        string `json:"blueprint_id"`
		ServiceID          string `json:"service_id"`
		SourcesOperation   string `json:"sources_operation"`
		InventoryOperation string `json:"inventory_operation"`
		Categories         []struct {
			ID           string `json:"id"`
			OwnerFolders bool   `json:"owner_folders"`
			Kinds        []struct {
				ID string `json:"id"`
			} `json:"kinds"`
		} `json:"categories"`
	}
	if err := json.Unmarshal(raw, &declaration); err != nil {
		t.Fatal(err)
	}
	// The categories that take the owner's own folders (§3.4, §4.5) are the
	// ones this service reads as folders, with the kinds their files list as;
	// the live category never declares it.
	for _, category := range declaration.Categories {
		kinds, reads := studioOwnerFolderKinds[category.ID]
		if category.OwnerFolders != reads {
			t.Errorf("category %s declares owner_folders=%v but the service reads folders of it: %v", category.ID, category.OwnerFolders, reads)
		}
		for _, kind := range kinds {
			if !slices.ContainsFunc(category.Kinds, func(declared struct {
				ID string `json:"id"`
			}) bool {
				return declared.ID == kind
			}) {
				t.Errorf("folder kind %s is not declared for %s", kind, category.ID)
			}
		}
	}
	var services []struct {
		ID         string            `json:"id"`
		Operations []json.RawMessage `json:"operations"`
	}
	if err := json.Unmarshal(manifest["services"], &services); err != nil {
		t.Fatal(err)
	}
	if len(services) != 1 || services[0].ID != declaration.ServiceID {
		t.Fatalf("services = %+v, declaration names %q", services, declaration.ServiceID)
	}
	// Both operations are declared byte-for-byte as Ori's operations.json
	// vectors: closed input and output schemas, read_only, no scopes, the
	// contract's timeout class and output bound.
	var want []json.RawMessage
	if err := json.Unmarshal(studioVector(t, "operations.json"), &want); err != nil {
		t.Fatal(err)
	}
	declaredOperations := map[string]any{}
	for _, operation := range services[0].Operations {
		var header struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(operation, &header); err != nil {
			t.Fatal(err)
		}
		declaredOperations[header.ID] = decodeAny(t, operation)
	}
	wanted := map[string]bool{}
	for _, operation := range want {
		var header struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(operation, &header); err != nil {
			t.Fatal(err)
		}
		wanted[header.ID] = true
		if !reflect.DeepEqual(declaredOperations[header.ID], decodeAny(t, operation)) {
			t.Errorf("operation %q differs from operations.json", header.ID)
		}
	}
	if !wanted[declaration.SourcesOperation] || !wanted[declaration.InventoryOperation] || declaration.SourcesOperation == declaration.InventoryOperation {
		t.Fatalf("declaration names %q and %q; vectors declare %v", declaration.SourcesOperation, declaration.InventoryOperation, wanted)
	}

	var blueprints []struct {
		ID           string   `json:"id"`
		Version      int      `json:"version"`
		Manifest     string   `json:"manifest"`
		Skeleton     string   `json:"skeleton"`
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal(manifest["blueprints"], &blueprints); err != nil {
		t.Fatal(err)
	}
	if len(blueprints) != 2 || blueprints[1].ID != declaration.BlueprintID || blueprints[1].Version != 1 ||
		blueprints[1].Manifest != "blueprints/reaper-studio/template.json" || blueprints[1].Skeleton != "blueprints/reaper-studio/workspace" ||
		blueprints[1].Capabilities == nil || len(blueprints[1].Capabilities) != 0 {
		t.Fatalf("blueprints = %+v", blueprints)
	}

	var features []string
	if err := json.Unmarshal(manifest["requires_host_features"], &features); err != nil {
		t.Fatal(err)
	}
	// 7 of the 8 allowed; resource_workspaces_v1 gates every Studio key, so an
	// older Ori refuses the whole contribution and keeps 0.10.1.
	if !slices.Equal(features, []string{"independent_program_homes_v1", "specialist_setup_journey_v1", "setup_quests_v2", "template_group_requirements_v1", "blueprint_inputs_v1", "home_profile_v1", "resource_workspaces_v1"}) {
		t.Fatalf("requires_host_features = %v", features)
	}
}

// The Studio team declaration is contract §3.3 to the letter: the vector with
// this plugin's own bounded system prompt and display words. Everything a
// resource team may not declare is absent, and the skeleton holds no project
// entry.
func TestStudioTemplateIsTheContractTeamWithABoundedPrompt(t *testing.T) {
	root := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(root, "blueprints", "reaper-studio", "template.json"))
	if err != nil {
		t.Fatal(err)
	}
	var template map[string]any
	if err := json.Unmarshal(data, &template); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"project_entry", "project_connection", "inputs", "starter_tasks", "setup_quest", "runtime_requirements", "setup_wizard", "standalone_composition", "intake", "agents", "capabilities"} {
		if _, declared := template[forbidden]; declared {
			t.Errorf("a resource team template declares %q", forbidden)
		}
	}
	name, _ := template["name"].(string)
	description, _ := template["description"].(string)
	if name != "REAPER Studio" || description == "" || len(description) > 240 || strings.ContainsAny(description, "\r\n") {
		t.Fatalf("name = %q, description = %q", name, description)
	}
	delete(template, "name")
	delete(template, "description")

	project, _ := template["assistant_project"].(map[string]any)
	roles, _ := project["roles"].([]any)
	if len(roles) != 1 {
		t.Fatalf("roles = %v", roles)
	}
	role, _ := roles[0].(map[string]any)
	prompt, _ := role["system_prompt"].(string)
	if len(prompt) < 200 || len(prompt) > 2000 || strings.ContainsAny(prompt, "\r\n") {
		t.Fatalf("system prompt is not one bounded line: %d bytes", len(prompt))
	}
	for _, phrase := range []string{"studio_url", "licensed, loaded, working or tested", "change nothing in REAPER", "only the owner saves annotations", "when an entry was seen"} {
		if !strings.Contains(prompt, phrase) {
			t.Errorf("system prompt lacks %q", phrase)
		}
	}
	for _, forbidden := range []string{"type", "scope", "capability_id"} {
		if _, declared := role[forbidden]; declared {
			t.Errorf("role declares the retired key %q", forbidden)
		}
	}

	want, _ := decodeAny(t, studioVector(t, "studio-template.valid.json")).(map[string]any)
	wantProject, _ := want["assistant_project"].(map[string]any)
	wantRoles, _ := wantProject["roles"].([]any)
	wantRole, _ := wantRoles[0].(map[string]any)
	wantRole["system_prompt"] = prompt
	if !reflect.DeepEqual(template, want) {
		t.Fatalf("template differs from studio-template.valid.json beyond the system prompt:\n%s", data)
	}
	group, _ := template["group_requirement"].(map[string]any)
	if _, declared := group["default_home_name"]; declared {
		t.Fatal("existing_only takes no default_home_name")
	}

	skeleton := filepath.Join(root, "blueprints", "reaper-studio", "workspace")
	files := 0
	err = filepath.WalkDir(skeleton, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		files++
		if strings.EqualFold(filepath.Ext(path), ".rpp") || entry.Type()&fs.ModeSymlink != 0 {
			t.Errorf("skeleton contains a project entry or link: %s", path)
		}
		return nil
	})
	if err != nil || files == 0 {
		t.Fatalf("skeleton walk: %v, %d files", err, files)
	}
	readme, err := os.ReadFile(filepath.Join(skeleton, "README.md"))
	if err != nil || !strings.Contains(string(readme), "not a song") {
		t.Fatalf("skeleton README: %v", err)
	}
}

// reaper-song (v10, reaper-song-team v1) stays byte-identical in this release
// (contract §3.3): Music's Home allowlists exactly that declaration.
func TestReaperSongTemplateStaysByteIdentical(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "blueprints", "reaper-song", "template.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != "71f1cfac411d111d48b41512b4cf54ec70d11ff1c92d6c10d199ba3eb94a88f5" {
		t.Fatalf("reaper-song/template.json changed: sha256 %s", got)
	}
}

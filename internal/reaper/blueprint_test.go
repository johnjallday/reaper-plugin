package reaper

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type reaperPluginBlueprintManifest struct {
	RequiresHostFeatures []string `json:"requires_host_features"`
	Blueprints           []struct {
		ID           string   `json:"id"`
		Version      int      `json:"version"`
		Manifest     string   `json:"manifest"`
		Skeleton     string   `json:"skeleton"`
		Capabilities []string `json:"capabilities"`
	} `json:"blueprints"`
}

type reaperSongTemplate struct {
	GroupRequirement struct {
		SchemaVersion      int    `json:"schema_version"`
		Policy             string `json:"policy"`
		AssistantProgramID string `json:"assistant_program_id"`
		MissingHome        string `json:"missing_home"`
		DefaultHomeName    string `json:"default_home_name"`
	} `json:"group_requirement"`
	StandaloneComposition struct {
		SchemaVersion int `json:"schema_version"`
		ProjectRoles  []struct {
			RoleID       string `json:"role_id"`
			SystemPrompt string `json:"system_prompt"`
		} `json:"project_roles"`
	} `json:"standalone_composition"`
	ProjectConnection struct {
		SchemaVersion  int      `json:"schema_version"`
		SupportedModes []string `json:"supported_modes"`
		AttachExisting struct {
			EntryExtensions []string `json:"entry_extensions"`
		} `json:"attach_existing"`
	} `json:"project_connection"`
	StarterTasks []struct {
		Description     string   `json:"description"`
		ConnectionModes []string `json:"connection_modes"`
	} `json:"starter_tasks"`
	Tools struct {
		Skills []string `json:"skills"`
	} `json:"tools"`
	Agents []struct {
		Tools struct {
			Skills []string `json:"skills"`
		} `json:"tools"`
	} `json:"agents"`
	RuntimeRequirements struct {
		Requirements []struct {
			Key        string `json:"key"`
			Disclosure string `json:"disclosure"`
			Adapter    string `json:"adapter"`
		} `json:"requirements"`
	} `json:"runtime_requirements"`
	AssistantProgram struct {
		SchemaVersion                  int      `json:"schema_version"`
		ID                             string   `json:"id"`
		StationName                    string   `json:"station_name"`
		DefaultPrimaryName             string   `json:"default_primary_name"`
		SuggestionRequiredCapabilities []string `json:"suggestion_required_capabilities"`
		Roles                          []struct {
			ID           string   `json:"id"`
			Label        string   `json:"label"`
			Scope        string   `json:"scope"`
			Required     bool     `json:"required"`
			CapabilityID string   `json:"capability_id"`
			Primary      bool     `json:"primary"`
			SystemPrompt string   `json:"system_prompt"`
			Skills       []string `json:"skills"`
		} `json:"roles"`
		Stages []struct {
			ID                          string `json:"id"`
			AcceptedCompletionThreshold int    `json:"accepted_completion_threshold"`
		} `json:"stages"`
		Reflection struct {
			MinimumProjects int    `json:"minimum_projects"`
			Rubric          string `json:"rubric"`
		} `json:"reflection"`
	} `json:"assistant_program"`
}

func TestReaperSongBlueprintV7RequiresReviewedHomeAndDeclaresStandaloneCustomization(t *testing.T) {
	root := filepath.Join("..", "..")
	manifestData, err := os.ReadFile(filepath.Join(root, ".ori-plugin", "plugin.json")) // #nosec G304 -- fixed repository fixture
	if err != nil {
		t.Fatal(err)
	}
	var manifest reaperPluginBlueprintManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(manifest.RequiresHostFeatures, []string{"assistant_program_v1", "specialist_setup_journey_v1", "setup_quests_v2", "template_group_requirements_v1"}) {
		t.Fatalf("requires_host_features = %v", manifest.RequiresHostFeatures)
	}
	if len(manifest.Blueprints) != 1 {
		t.Fatalf("blueprints = %+v", manifest.Blueprints)
	}
	blueprint := manifest.Blueprints[0]
	if blueprint.ID != "reaper-song" || blueprint.Version != 7 ||
		blueprint.Manifest != "blueprints/reaper-song/template.json" ||
		blueprint.Skeleton != "blueprints/reaper-song/project" ||
		!slices.Equal(blueprint.Capabilities, []string{"reaper-live-control"}) {
		t.Fatalf("blueprint contribution = %+v", blueprint)
	}

	templateData, err := os.ReadFile(filepath.Join(root, blueprint.Manifest)) // #nosec G304 -- manifest-fixed repository fixture
	if err != nil {
		t.Fatal(err)
	}
	var template reaperSongTemplate
	if err := json.Unmarshal(templateData, &template); err != nil {
		t.Fatal(err)
	}
	if template.GroupRequirement.SchemaVersion != 1 || template.GroupRequirement.Policy != "required" ||
		template.GroupRequirement.AssistantProgramID != "music-producer-assistant" || template.GroupRequirement.MissingHome != "offer_create" ||
		template.GroupRequirement.DefaultHomeName != "Music Production Home" {
		t.Fatalf("group requirement = %+v", template.GroupRequirement)
	}
	if template.StandaloneComposition.SchemaVersion != 1 || len(template.StandaloneComposition.ProjectRoles) != 3 {
		t.Fatalf("standalone composition = %+v", template.StandaloneComposition)
	}
	for index, roleID := range []string{"producer", "engineer", "songwriter"} {
		role := template.StandaloneComposition.ProjectRoles[index]
		if role.RoleID != roleID || !strings.Contains(role.SystemPrompt, "this one REAPER") ||
			!strings.Contains(role.SystemPrompt, "no Home") && !strings.Contains(role.SystemPrompt, "no Assistant Program Home") {
			t.Fatalf("standalone role %q is not project-local: %+v", roleID, role)
		}
		for _, forbidden := range []string{"linked projects", "Music Production Home", "Collaborator stage"} {
			if strings.Contains(role.SystemPrompt, forbidden) {
				t.Fatalf("standalone role %q claims grouped scope: %q", roleID, role.SystemPrompt)
			}
		}
	}
	if template.ProjectConnection.SchemaVersion != 1 ||
		!slices.Equal(template.ProjectConnection.SupportedModes, []string{"new_project", "existing_project"}) ||
		!slices.Equal(template.ProjectConnection.AttachExisting.EntryExtensions, []string{".rpp"}) {
		t.Fatalf("project connection declaration = %+v", template.ProjectConnection)
	}
	if len(template.StarterTasks) != 2 ||
		!slices.Equal(template.StarterTasks[0].ConnectionModes, []string{"new_project"}) ||
		!slices.Equal(template.StarterTasks[1].ConnectionModes, []string{"new_project", "existing_project"}) {
		t.Fatalf("starter task connection modes = %+v", template.StarterTasks)
	}
	for _, skill := range []string{"reaper-session-setup", "reaper-web-remote", "reaper-project-tidy"} {
		if !slices.Contains(template.Tools.Skills, skill) {
			t.Errorf("template does not bind %s", skill)
		}
	}
	if len(template.Agents) != 0 {
		t.Fatalf("assistant roster must be hired from the station, got legacy seeds: %+v", template.Agents)
	}
	program := template.AssistantProgram
	if program.SchemaVersion != 2 || program.ID != "music-producer-assistant" ||
		program.StationName != "Music Production Home" || program.DefaultPrimaryName != "Portfolio Manager" ||
		!slices.Equal(program.SuggestionRequiredCapabilities, []string{"reaper_live_control"}) {
		t.Fatalf("assistant program identity = %+v", program)
	}
	if len(program.Roles) != 5 || program.Roles[0].ID != "portfolio_manager" ||
		program.Roles[0].Scope != "home" || !program.Roles[0].Required || !program.Roles[0].Primary ||
		program.Roles[1].ID != "producer" || program.Roles[1].Scope != "project" ||
		!program.Roles[1].Required || !program.Roles[1].Primary ||
		program.Roles[2].ID != "engineer" || program.Roles[2].Scope != "project" || !program.Roles[2].Required ||
		program.Roles[3].ID != "songwriter" || program.Roles[3].Scope != "project" || !program.Roles[3].Required ||
		program.Roles[4].ID != "sample_library_manager" || program.Roles[4].Scope != "home" ||
		program.Roles[4].Required || program.Roles[4].CapabilityID != "sample-library" {
		t.Fatalf("assistant scoped roles = %+v", program.Roles)
	}
	if !slices.Contains(program.Roles[1].Skills, "reaper-project-tidy") ||
		!strings.Contains(program.Roles[1].SystemPrompt, "required_capabilities: [reaper_live_control]") ||
		!strings.Contains(program.Roles[2].SystemPrompt, "Return composition") ||
		!strings.Contains(program.Roles[3].SystemPrompt, "Return mixing") {
		t.Fatalf("assistant role boundaries or gates are incomplete: %+v", program.Roles)
	}
	if len(program.Stages) != 2 || program.Stages[0].ID != "helper" || program.Stages[0].AcceptedCompletionThreshold != 0 ||
		program.Stages[1].ID != "collaborator" || program.Stages[1].AcceptedCompletionThreshold != 5 {
		t.Fatalf("assistant stages = %+v", program.Stages)
	}
	if program.Reflection.MinimumProjects != 3 || !strings.Contains(program.Reflection.Rubric, "three distinct linked projects") {
		t.Fatalf("assistant reflection policy = %+v", program.Reflection)
	}
	if len(template.RuntimeRequirements.Requirements) != 1 {
		t.Fatalf("runtime requirements = %+v", template.RuntimeRequirements.Requirements)
	}
	requirement := template.RuntimeRequirements.Requirements[0]
	const disclosure = "The selected agent may contact REAPER over loopback and read and write the dedicated Ori REAPER runner directory without Ori's per-call confirmation while executing approved workspace tasks. Trusted runner scripts execute inside REAPER and can change the open project; Ori uses REAPER's undo mechanism where supported. You can revoke REAPER access from this workspace's live-control setup."
	if requirement.Key != "reaper_live_control" || requirement.Adapter != "plugin:reaper-plugin:reaper-runtime" || requirement.Disclosure != disclosure {
		t.Fatalf("live-control requirement changed: %+v", requirement)
	}

	projectRoot := filepath.Join(root, blueprint.Skeleton)
	instance := t.TempDir()
	instantiateBlueprintFixture(t, projectRoot, instance, "Neon Song")
	seeded, err := os.ReadFile(filepath.Join(instance, "conventions.md"))
	if err != nil {
		t.Fatalf("instantiated conventions: %v", err)
	}
	canonical, err := os.ReadFile(filepath.Join(projectRoot, "conventions.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(seeded) != string(canonical) || !strings.Contains(string(seeded), "ori-reaper-tidy-conventions:1") {
		t.Fatalf("instantiated conventions differ from canonical defaults")
	}
	if _, err := os.Stat(filepath.Join(instance, "Neon Song.rpp")); err != nil {
		t.Fatalf("instantiated project entry: %v", err)
	}
}

func instantiateBlueprintFixture(t *testing.T, source, destination, name string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil || relative == "." {
			return err
		}
		relative = strings.ReplaceAll(relative, "{{name}}", name)
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		data, err := os.ReadFile(path) // #nosec G304 -- path is from fixed blueprint fixture walk
		if err != nil {
			return err
		}
		data = []byte(strings.ReplaceAll(string(data), "{{name}}", name))
		return os.WriteFile(target, data, 0o600) // #nosec G306 -- private instantiated fixture
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Ori retired the agent Type field (johnjallday/ori-agent#490) and only keeps
// accepting the key so older manifests still decode. Declaring it again would
// hold Ori to that compatibility shim, so no role or agent may carry it. The
// check reads raw JSON because the typed fixture structs silently drop unknown
// keys.
func TestReaperSongBlueprintDeclaresNoRetiredAgentType(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "blueprints", "reaper-song", "template.json")) // #nosec G304 -- fixed repository fixture
	if err != nil {
		t.Fatal(err)
	}
	var template struct {
		Agents           []map[string]json.RawMessage `json:"agents"`
		AssistantProgram struct {
			Roles []map[string]json.RawMessage `json:"roles"`
		} `json:"assistant_program"`
	}
	if err := json.Unmarshal(data, &template); err != nil {
		t.Fatal(err)
	}
	if len(template.AssistantProgram.Roles) == 0 {
		t.Fatal("assistant_program.roles is empty; the guard would pass vacuously")
	}
	for _, role := range template.AssistantProgram.Roles {
		if _, ok := role["type"]; ok {
			t.Errorf("assistant_program role %s still declares the retired \"type\" key", role["id"])
		}
	}
	for index, agent := range template.Agents {
		if _, ok := agent["type"]; ok {
			t.Errorf("agents[%d] still declares the retired \"type\" key", index)
		}
	}
}

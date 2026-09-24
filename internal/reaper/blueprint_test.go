package reaper

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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
		AssistantProjectID string `json:"assistant_project_id"`
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
		Details         string   `json:"details"`
		ConnectionModes []string `json:"connection_modes"`
	} `json:"starter_tasks"`
	ProjectEntry struct {
		RelativePath string `json:"relative_path"`
	} `json:"project_entry"`
	Inputs struct {
		SchemaVersion int      `json:"schema_version"`
		Title         string   `json:"title"`
		ApplyTo       []string `json:"apply_to"`
		Fields        []struct {
			ID      string          `json:"id"`
			Label   string          `json:"label"`
			Type    string          `json:"type"`
			Unit    string          `json:"unit"`
			Min     float64         `json:"min"`
			Max     float64         `json:"max"`
			Step    float64         `json:"step"`
			Default json.RawMessage `json:"default"`
			Options []struct {
				Value string `json:"value"`
				Label string `json:"label"`
			} `json:"options"`
		} `json:"fields"`
	} `json:"inputs"`
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
	AssistantProject struct {
		SchemaVersion int    `json:"schema_version"`
		Version       int    `json:"version"`
		ID            string `json:"id"`
		Home          struct {
			ProviderPluginID  string `json:"provider_plugin_id"`
			ProgramID         string `json:"program_id"`
			HomeSchemaVersion int    `json:"home_schema_version"`
			MinHomeVersion    int    `json:"min_home_version"`
			MaxHomeVersion    int    `json:"max_home_version"`
		} `json:"home"`
		Roles []struct {
			ID           string   `json:"id"`
			Label        string   `json:"label"`
			Required     bool     `json:"required"`
			Primary      bool     `json:"primary"`
			SystemPrompt string   `json:"system_prompt"`
			Skills       []string `json:"skills"`
		} `json:"roles"`
	} `json:"assistant_project"`
}

func TestReaperSongBlueprintV10ReferencesIndependentHomeAndDeclaresStandaloneCustomization(t *testing.T) {
	root := filepath.Join("..", "..")
	manifestData, err := os.ReadFile(filepath.Join(root, ".ori-plugin", "plugin.json")) // #nosec G304 -- fixed repository fixture
	if err != nil {
		t.Fatal(err)
	}
	var manifest reaperPluginBlueprintManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	// independent_program_homes_v1 is the release gate for assistant_project;
	// blueprint_inputs_v1 remains the gate for the typed inputs below. A host
	// missing either feature must refuse this contribution before registration.
	if !slices.Equal(manifest.RequiresHostFeatures, []string{"independent_program_homes_v1", "specialist_setup_journey_v1", "setup_quests_v2", "template_group_requirements_v1", "blueprint_inputs_v1"}) {
		t.Fatalf("requires_host_features = %v", manifest.RequiresHostFeatures)
	}
	if len(manifest.Blueprints) != 1 {
		t.Fatalf("blueprints = %+v", manifest.Blueprints)
	}
	blueprint := manifest.Blueprints[0]
	if blueprint.ID != "reaper-song" || blueprint.Version != 10 ||
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
	if template.GroupRequirement.SchemaVersion != 2 || template.GroupRequirement.Policy != "required" ||
		template.GroupRequirement.AssistantProjectID != "reaper-song-team" || template.GroupRequirement.MissingHome != "offer_create" ||
		template.GroupRequirement.DefaultHomeName != "Music Production Home" {
		t.Fatalf("group requirement = %+v", template.GroupRequirement)
	}
	// v10 staffs one project role. The standalone variant carries exactly that
	// role's Home-free prompt and nothing for the retired three-role team.
	if template.StandaloneComposition.SchemaVersion != 2 || len(template.StandaloneComposition.ProjectRoles) != 1 {
		t.Fatalf("standalone composition = %+v", template.StandaloneComposition)
	}
	for index, roleID := range []string{"reaper-assistant"} {
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
	assertSessionInputsDeclaration(t, root, template)
	for _, skill := range []string{"reaper-session-setup", "reaper-web-remote", "reaper-project-tidy"} {
		if !slices.Contains(template.Tools.Skills, skill) {
			t.Errorf("template does not bind %s", skill)
		}
	}
	if len(template.Agents) != 0 {
		t.Fatalf("assistant roster must be hired from the station, got legacy seeds: %+v", template.Agents)
	}
	project := template.AssistantProject
	if project.SchemaVersion != 1 || project.Version != 1 || project.ID != "reaper-song-team" ||
		project.Home.ProviderPluginID != "music-project-management" || project.Home.ProgramID != "music-producer-assistant" ||
		project.Home.HomeSchemaVersion != 1 || project.Home.MinHomeVersion != 1 || project.Home.MaxHomeVersion != 1 {
		t.Fatalf("assistant project identity = %+v", project)
	}
	// The team keeps schema 1, version 1 so the published Music Project
	// Management Home (which authorizes team version 1 only) still admits it; the
	// role set itself is what v10 changes.
	if len(project.Roles) != 1 || project.Roles[0].ID != "reaper-assistant" || project.Roles[0].Label != "REAPER Assistant" ||
		!project.Roles[0].Required || !project.Roles[0].Primary {
		t.Fatalf("assistant project roles = %+v", project.Roles)
	}
	// One role owns the whole REAPER scope: the mixing/recording side the Mix
	// Engineer used to hold and the arrangement side the Songwriter used to hold,
	// with nobody left to delegate to. The live-control gate is unchanged.
	assistant := project.Roles[0]
	if !slices.Contains(assistant.Skills, "reaper-project-tidy") ||
		!strings.Contains(assistant.SystemPrompt, "required_capabilities: [reaper_live_control]") ||
		!strings.Contains(assistant.SystemPrompt, "mixing") || !strings.Contains(assistant.SystemPrompt, "arrangement") ||
		!strings.Contains(assistant.SystemPrompt, "no other project roles") {
		t.Fatalf("assistant scope or gates are incomplete: %+v", assistant)
	}
	for _, retired := range []string{"Producer", "Mix Engineer", "Songwriter"} {
		if strings.Contains(assistant.SystemPrompt, retired) {
			t.Fatalf("assistant prompt still refers to the retired team (%q): %q", retired, assistant.SystemPrompt)
		}
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
func TestReaperSongBlueprintOwnsOnlyProjectRolesAndDeclaresNoRetiredAgentType(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "blueprints", "reaper-song", "template.json")) // #nosec G304 -- fixed repository fixture
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, combined := raw["assistant_program"]; combined {
		t.Fatal("REAPER still owns a combined Assistant Program")
	}
	var template struct {
		Agents           []map[string]json.RawMessage `json:"agents"`
		AssistantProject struct {
			Roles []map[string]json.RawMessage `json:"roles"`
		} `json:"assistant_project"`
	}
	if err := json.Unmarshal(data, &template); err != nil {
		t.Fatal(err)
	}
	if len(template.AssistantProject.Roles) != 1 {
		t.Fatalf("assistant_project.roles = %+v", template.AssistantProject.Roles)
	}
	for _, role := range template.AssistantProject.Roles {
		for _, forbidden := range []string{"type", "scope", "capability_id"} {
			if _, ok := role[forbidden]; ok {
				t.Errorf("assistant_project role %s declares forbidden %q", role["id"], forbidden)
			}
		}
		switch id := string(role["id"]); id {
		case `"portfolio_manager"`, `"sample_library_manager"`:
			t.Errorf("REAPER still claims Home role %s", id)
		case `"producer"`, `"engineer"`, `"songwriter"`:
			t.Errorf("REAPER still declares retired project role %s", id)
		}
		var skills []string
		if err := json.Unmarshal(role["skills"], &skills); err != nil && len(role["skills"]) != 0 {
			t.Fatalf("role %s skills: %v", role["id"], err)
		}
		if slices.Contains(skills, "music-project-management") {
			t.Errorf("REAPER role %s bundles the Home provider skill", role["id"])
		}
	}
	for index, agent := range template.Agents {
		if _, ok := agent["type"]; ok {
			t.Errorf("agents[%d] still declares the retired \"type\" key", index)
		}
	}
}

// assertSessionInputsDeclaration pins the v8 contract: the two values the
// blueprint asks for at creation, the one file they reach, and the tokens that
// file actually uses. Nothing here may become free text — a user's answer ends
// up inside the session file, so "number in a range" and "one of these
// options" are the whole of the safety argument.
func assertSessionInputsDeclaration(t *testing.T, root string, template reaperSongTemplate) {
	t.Helper()
	inputs := template.Inputs
	if inputs.SchemaVersion != 1 || inputs.Title != "Session settings" {
		t.Fatalf("inputs declaration = %+v", inputs)
	}
	// Only the scaffolded session file is rewritten, and it is the same file the
	// blueprint already names as its project entry.
	if !slices.Equal(inputs.ApplyTo, []string{"{{name}}.rpp"}) ||
		template.ProjectEntry.RelativePath != "{{name}}.rpp" {
		t.Fatalf("apply_to = %v, project entry = %q", inputs.ApplyTo, template.ProjectEntry.RelativePath)
	}
	if len(inputs.Fields) != 2 {
		t.Fatalf("fields = %+v", inputs.Fields)
	}

	tempo := inputs.Fields[0]
	if tempo.ID != "tempo" || tempo.Label != "Tempo" || tempo.Type != "number" || tempo.Unit != "BPM" ||
		tempo.Min != 40 || tempo.Max != 240 || tempo.Step != 1 || string(tempo.Default) != "120" ||
		len(tempo.Options) != 0 {
		t.Fatalf("tempo field = %+v", tempo)
	}

	signature := inputs.Fields[1]
	if signature.ID != "time_signature" || signature.Label != "Time signature" ||
		signature.Type != "select" || string(signature.Default) != `"4 4"` || len(signature.Options) != 3 {
		t.Fatalf("time signature field = %+v", signature)
	}
	// The option values are written verbatim into the session file's TEMPO
	// line, so they must be the beats/unit pair REAPER expects, not the display
	// text a person reads.
	for index, want := range [][2]string{{"4 4", "4/4"}, {"3 4", "3/4"}, {"6 8", "6/8"}} {
		option := signature.Options[index]
		if option.Value != want[0] || option.Label != want[1] {
			t.Fatalf("option %d = %+v, want %v", index, option, want)
		}
	}

	scaffold, err := os.ReadFile(filepath.Join(root, "blueprints", "reaper-song", "project", "{{name}}.rpp")) // #nosec G304 -- fixed repository fixture
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(scaffold), "TEMPO {{input.tempo}} {{input.time_signature}}") {
		t.Fatalf("the scaffold does not use the declared tokens:\n%s", scaffold)
	}
	// A token no field declares would survive into the created project, so the
	// blueprint must use only ids it declared.
	declared := map[string]bool{}
	for _, field := range inputs.Fields {
		declared[field.ID] = true
	}
	for _, token := range regexp.MustCompile(`\{\{input\.([^}]*)\}\}`).FindAllStringSubmatch(string(scaffold), -1) {
		if !declared[token[1]] {
			t.Fatalf("the scaffold uses {{input.%s}}, which no field declares", token[1])
		}
	}

	// Requirement 47: the first starter task no longer states the session's
	// tempo as a fact, still offers the key, and substitutes no values.
	details := template.StarterTasks[0].Details
	if strings.Contains(details, "120 BPM") || strings.Contains(details, "{{input.") {
		t.Fatalf("the starter task asserts creation values or carries a token:\n%s", details)
	}
	for _, phrase := range []string{"chose when the workspace was created", "key"} {
		if !strings.Contains(details, phrase) {
			t.Fatalf("the starter task does not mention %q:\n%s", phrase, details)
		}
	}
}

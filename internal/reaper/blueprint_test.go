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
		ID                             string   `json:"id"`
		StationName                    string   `json:"station_name"`
		DefaultPrimaryName             string   `json:"default_primary_name"`
		SuggestionRequiredCapabilities []string `json:"suggestion_required_capabilities"`
		Roles                          []struct {
			ID           string   `json:"id"`
			Label        string   `json:"label"`
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

func TestReaperSongBlueprintV3DeclaresSharedProducerProgram(t *testing.T) {
	root := filepath.Join("..", "..")
	manifestData, err := os.ReadFile(filepath.Join(root, ".ori-plugin", "plugin.json")) // #nosec G304 -- fixed repository fixture
	if err != nil {
		t.Fatal(err)
	}
	var manifest reaperPluginBlueprintManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(manifest.RequiresHostFeatures, []string{"assistant_program_v1"}) {
		t.Fatalf("requires_host_features = %v", manifest.RequiresHostFeatures)
	}
	if len(manifest.Blueprints) != 1 {
		t.Fatalf("blueprints = %+v", manifest.Blueprints)
	}
	blueprint := manifest.Blueprints[0]
	if blueprint.ID != "reaper-song" || blueprint.Version != 3 ||
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
	for _, skill := range []string{"reaper-session-setup", "reaper-web-remote", "reaper-project-tidy"} {
		if !slices.Contains(template.Tools.Skills, skill) {
			t.Errorf("template does not bind %s", skill)
		}
	}
	if len(template.Agents) != 0 {
		t.Fatalf("assistant roster must be hired from the station, got legacy seeds: %+v", template.Agents)
	}
	program := template.AssistantProgram
	if program.ID != "music-producer-assistant" || program.StationName != "Producer Home" || program.DefaultPrimaryName != "Producer" ||
		!slices.Equal(program.SuggestionRequiredCapabilities, []string{"reaper_live_control"}) {
		t.Fatalf("assistant program identity = %+v", program)
	}
	if len(program.Roles) != 3 || program.Roles[0].ID != "producer" || !program.Roles[0].Primary ||
		program.Roles[1].ID != "engineer" || program.Roles[2].ID != "songwriter" {
		t.Fatalf("assistant roles = %+v", program.Roles)
	}
	if !slices.Contains(program.Roles[0].Skills, "reaper-project-tidy") ||
		!strings.Contains(program.Roles[0].SystemPrompt, "required_capabilities: [reaper_live_control]") ||
		!strings.Contains(program.Roles[1].SystemPrompt, "Return composition") ||
		!strings.Contains(program.Roles[2].SystemPrompt, "Return mixing") {
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

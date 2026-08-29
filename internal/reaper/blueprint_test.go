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
	Blueprints []struct {
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
}

func TestReaperSongBlueprintV2SeedsConventionsAndPreservesLiveControlDisclosure(t *testing.T) {
	root := filepath.Join("..", "..")
	manifestData, err := os.ReadFile(filepath.Join(root, ".ori-plugin", "plugin.json")) // #nosec G304 -- fixed repository fixture
	if err != nil {
		t.Fatal(err)
	}
	var manifest reaperPluginBlueprintManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Blueprints) != 1 {
		t.Fatalf("blueprints = %+v", manifest.Blueprints)
	}
	blueprint := manifest.Blueprints[0]
	if blueprint.ID != "reaper-song" || blueprint.Version != 2 ||
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
	if len(template.Agents) != 1 || !slices.Contains(template.Agents[0].Tools.Skills, "reaper-project-tidy") {
		t.Fatalf("producer does not bind tidy skill: %+v", template.Agents)
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

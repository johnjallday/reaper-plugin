package reaper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultTidyConventionsMatchBlueprintAndParse(t *testing.T) {
	blueprint, err := os.ReadFile(filepath.Join("..", "..", "blueprints", "reaper-song", "project", "conventions.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(DefaultTidyConventions()) != string(blueprint) {
		t.Fatalf("embedded old-workspace seed differs from new-workspace blueprint")
	}
	conventions, err := ParseTidyConventions(blueprint)
	if err != nil {
		t.Fatalf("ParseTidyConventions() error = %v", err)
	}
	if len(conventions.Roles) != 7 || conventions.MarkerCase != "Title Case" ||
		conventions.Numbering != "Space Before Number" || len(conventions.SectionNames) < 5 {
		t.Fatalf("default conventions = %+v", conventions)
	}
}

func TestLoadTidyConventionsSeedsMissingAndFallsBackForUnusableFiles(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		root := t.TempDir()
		loaded, err := LoadTidyConventions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !loaded.Seeded || loaded.UsedDefault || !strings.Contains(loaded.Note, "Created conventions.md") {
			t.Fatalf("load = %+v", loaded)
		}
		path := filepath.Join(root, "conventions.md")
		data, err := os.ReadFile(path)
		if err != nil || string(data) != string(DefaultTidyConventions()) {
			t.Fatalf("seed = %q, %v", data, err)
		}
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("seed permissions = %+v, %v", info, err)
		}
	})

	for name, body := range map[string]string{"empty": "", "malformed": "# not the contract\nvocals = blue\n"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "conventions.md")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadTidyConventions(root)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Seeded || !loaded.UsedDefault || !strings.Contains(loaded.Note, "used the Reaper Song defaults") {
				t.Fatalf("load = %+v", loaded)
			}
			persisted, err := os.ReadFile(path)
			if err != nil || string(persisted) != body {
				t.Fatalf("fallback overwrote user file = %q, %v", persisted, err)
			}
		})
	}
}

func TestGenerateTidySurveyUsesConventionsAndOnlyExactDuplicates(t *testing.T) {
	conventions, err := ParseTidyConventions(DefaultTidyConventions())
	if err != nil {
		t.Fatal(err)
	}
	state := messyTidySurveyState()
	proposal, err := GenerateTidySurvey(state, conventions, "survey-001", "")
	if err != nil {
		t.Fatalf("GenerateTidySurvey() error = %v", err)
	}
	if proposal.AlreadyTidy || proposal.Plan == nil || proposal.Plan.Validate() != nil || len(proposal.Plan.Items) != 7 {
		t.Fatalf("proposal = %+v", proposal)
	}
	items := make(map[string]TidyPlanItem)
	for _, item := range proposal.Plan.Items {
		items[item.ID] = item
		if strings.TrimSpace(item.Reason) == "" {
			t.Fatalf("item has no convention/duplicate reason: %+v", item)
		}
	}
	vocal := items["color-track-db7f23bb38b638448a8722fd017ee05b"]
	if vocal.Payload.Color == nil || *vocal.Payload.Color != (TidyRGB{Red: 55, Green: 126, Blue: 230}) {
		t.Fatalf("vocal color item = %+v", vocal)
	}
	drums := items["color-track-771c2680b9fa2b48ad94adffa0a2a90c"]
	if drums.Payload.Color == nil || *drums.Payload.Color != (TidyRGB{Red: 220, Green: 68, Blue: 64}) {
		t.Fatalf("drum bus role item = %+v", drums)
	}
	for _, item := range proposal.Plan.Items {
		if item.Target.TrackGUID == "{AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA}" {
			t.Fatalf("unknown track role was colored: %+v", item)
		}
	}
	if _, exists := items["delete-marker-2"]; !exists {
		t.Fatalf("same-name exact duplicate was not proposed")
	}
	if _, exists := items["delete-marker-3"]; !exists {
		t.Fatalf("empty-name exact duplicate was not proposed")
	}
	if _, exists := items["delete-marker-4"]; exists {
		t.Fatalf("+0.5ms marker was treated as an exact duplicate")
	}
	if rename := items["rename-region-202"]; rename.Payload.NewName == nil || *rename.Payload.NewName != "Chorus 2" {
		t.Fatalf("region rename = %+v", rename)
	}
	if len(proposal.Lines) != len(proposal.Plan.Items) {
		t.Fatalf("human lines = %d, plan items = %d", len(proposal.Lines), len(proposal.Plan.Items))
	}
}

func TestGenerateTidySurveyCustomConventionChangesProposal(t *testing.T) {
	custom := strings.Replace(string(DefaultTidyConventions()), "vocals = 55, 126, 230", "vocals = 1, 2, 3", 1)
	conventions, err := ParseTidyConventions([]byte(custom))
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := GenerateTidySurvey(messyTidySurveyState(), conventions, "survey-custom", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range proposal.Plan.Items {
		if item.Target.TrackGUID == tidyTestGUID {
			if item.Payload.Color == nil || *item.Payload.Color != (TidyRGB{Red: 1, Green: 2, Blue: 3}) ||
				!strings.Contains(item.Reason, "rgb(1, 2, 3)") {
				t.Fatalf("custom vocal item = %+v", item)
			}
			return
		}
	}
	t.Fatalf("custom vocal proposal was not generated")
}

func TestGenerateTidySurveyAlreadyTidyAndUnknownRolesAreValidNoChangeOutcomes(t *testing.T) {
	conventions, err := ParseTidyConventions(DefaultTidyConventions())
	if err != nil {
		t.Fatal(err)
	}
	state := messyTidySurveyState()
	state.Tracks[0].Color = tidyColor(55, 126, 230)
	state.Tracks[1].Color = tidyColor(220, 68, 64)
	state.Markers = []TidyInspectedMarker{
		{EnumerationIndex: 0, ID: 1, PositionSeconds: 10, Name: "Chorus"},
		{EnumerationIndex: 1, ID: 4, PositionSeconds: 10.0005, Name: "Chorus"},
		{EnumerationIndex: 2, ID: 202, IsRegion: true, PositionSeconds: 30, EndSeconds: tidyFloat(40), Name: "Chorus 2"},
	}
	proposal, err := GenerateTidySurvey(state, conventions, "already-tidy", "")
	if err != nil {
		t.Fatal(err)
	}
	if !proposal.AlreadyTidy || proposal.Plan != nil || len(proposal.Lines) != 0 {
		t.Fatalf("already-tidy proposal = %+v", proposal)
	}

	state.Tracks[2].Name = "Vox Guitar"
	proposal, err = GenerateTidySurvey(state, conventions, "ambiguous-role", "")
	if err != nil || !proposal.AlreadyTidy {
		t.Fatalf("ambiguous role should remain uncolored: %+v, %v", proposal, err)
	}
}

func messyTidySurveyState() TidyInspectedState {
	return TidyInspectedState{
		SchemaVersion: TidySchemaVersion,
		Project:       TidyInspectedProject{Name: "Messy Project", Path: "/tmp/messy.RPP", SaveDirty: true, ProjectChangeCount: 10},
		Tracks: []TidyInspectedTrack{
			{GUID: tidyTestGUID, Index: 0, Name: "Vox 2"},
			{GUID: "{771C2680-B9FA-2B48-AD94-ADFFA0A2A90C}", Index: 1, Name: "DRUM BUS"},
			{GUID: "{AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA}", Index: 2, Name: "Mystery"},
		},
		Markers: []TidyInspectedMarker{
			{EnumerationIndex: 0, ID: 1, PositionSeconds: 10, Name: "chorus"},
			{EnumerationIndex: 1, ID: 2, PositionSeconds: 10, Name: "chorus"},
			{EnumerationIndex: 2, ID: 3, PositionSeconds: 10, Name: ""},
			{EnumerationIndex: 3, ID: 4, PositionSeconds: 10.0005, Name: "chorus"},
			{EnumerationIndex: 4, ID: 202, IsRegion: true, PositionSeconds: 30, EndSeconds: tidyFloat(40), Name: "chorus2"},
		},
	}
}

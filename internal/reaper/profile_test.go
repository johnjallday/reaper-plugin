package reaper

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type profileProbeStub struct {
	result ProfileResult
	asked  []bool
}

func (p *profileProbeStub) ReadProfile(_ context.Context, includeTemplates bool) ProfileResult {
	p.asked = append(p.asked, includeTemplates)
	return p.result
}

func writeTemplate(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("<REAPER_PROJECT\n>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// listIn lists the templates of the resource folder at dir, the way the
// platform reader does once it holds that folder open.
func listIn(t *testing.T, dir string) (templates []ProfileTemplate, available, truncated bool) {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if listed := profileTemplatesAvailable(root); listed != func() bool {
		_, has, _ := listProfileTemplates(root)
		return has
	}() {
		t.Fatalf("the availability check (%v) disagrees with the listing", listed)
	}
	return listProfileTemplates(root)
}

func templateNames(templates []ProfileTemplate) []string {
	names := make([]string, 0, len(templates))
	for _, template := range templates {
		names = append(names, template.Kind+":"+template.Name)
	}
	return names
}

func TestListProfileTemplatesNamesOnlyTheTemplateFilesInBothFolders(t *testing.T) {
	resource := t.TempDir()
	projects := filepath.Join(resource, "ProjectTemplates")
	tracks := filepath.Join(resource, "TrackTemplates")
	writeTemplate(t, projects, "Vocal Comp.RPP")
	writeTemplate(t, projects, "band session.rpp")
	writeTemplate(t, tracks, "Drum Bus.RTrackTemplate")
	// None of these is a template of that folder, and none makes the list
	// incomplete.
	writeTemplate(t, projects, "Vocal Comp.RPP-bak")
	writeTemplate(t, projects, "notes.txt")
	writeTemplate(t, projects, ".hidden.RPP")
	writeTemplate(t, projects, "Drum Bus.RTrackTemplate")
	writeTemplate(t, tracks, "Session.RPP")
	if err := os.Mkdir(filepath.Join(projects, ".cache"), 0o700); err != nil {
		t.Fatal(err)
	}

	templates, available, truncated := listIn(t, resource)
	if !available || truncated {
		t.Fatalf("available = %v truncated = %v", available, truncated)
	}
	// Project templates first, then track templates, each in name order
	// without regard to case.
	want := []string{"project:band session", "project:Vocal Comp", "track:Drum Bus"}
	if got := templateNames(templates); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("templates = %v, want %v", got, want)
	}
	first := templates[0]
	if first.File != "band session.rpp" || first.ModifiedAt == "" {
		t.Fatalf("first template = %+v", first)
	}
	if _, err := time.Parse(time.RFC3339, first.ModifiedAt); err != nil || !strings.HasSuffix(first.ModifiedAt, "Z") {
		t.Fatalf("modified_at = %q: %v", first.ModifiedAt, err)
	}
	// Nothing returned can be read as a path.
	encoded, err := json.Marshal(templates)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), resource) || strings.Contains(string(encoded), "/") {
		t.Fatalf("the listing carries a path: %s", encoded)
	}
}

// What a user would call their templates but this read does not name is never
// passed over in silence: the list says it is not everything.
func TestListProfileTemplatesSaysWhenTheListIsNotEverything(t *testing.T) {
	outside := t.TempDir()
	writeTemplate(t, outside, "Outside.RPP")
	tests := map[string]func(t *testing.T, projects string){
		"a subfolder, whose templates are not listed": func(t *testing.T, projects string) {
			writeTemplate(t, filepath.Join(projects, "Bands"), "Nested.RPP")
		},
		"a folder named like a template": func(t *testing.T, projects string) {
			if err := os.Mkdir(filepath.Join(projects, "Folder.RPP"), 0o700); err != nil {
				t.Fatal(err)
			}
		},
		"a template that is a link": func(t *testing.T, projects string) {
			if err := os.Symlink(filepath.Join(outside, "Outside.RPP"), filepath.Join(projects, "Linked.RPP")); err != nil {
				t.Fatal(err)
			}
		},
		"a name longer than a host can show": func(t *testing.T, projects string) {
			writeTemplate(t, projects, strings.Repeat("n", maxProfileTemplateName+1)+".RPP")
		},
		"a name of two lines": func(t *testing.T, projects string) {
			writeTemplate(t, projects, "Two\nlines.RPP")
		},
		"a name with nothing in it": func(t *testing.T, projects string) {
			writeTemplate(t, projects, " .RPP")
		},
		"a name that reads backwards": func(t *testing.T, projects string) {
			writeTemplate(t, projects, "Mix"+string(rune(0x202e))+"PPR.exe.RPP")
		},
		"a name that is invisible": func(t *testing.T, projects string) {
			writeTemplate(t, projects, string(rune(0x200b))+".RPP")
		},
	}
	for name, add := range tests {
		t.Run(name, func(t *testing.T) {
			resource := t.TempDir()
			projects := filepath.Join(resource, "ProjectTemplates")
			writeTemplate(t, projects, "Good.RPP")
			add(t, projects)
			templates, available, truncated := listIn(t, resource)
			if !available || !truncated || len(templates) != 1 || templates[0].Name != "Good" {
				t.Fatalf("templates = %v available = %v truncated = %v", templateNames(templates), available, truncated)
			}
		})
	}
}

// A folder that is there and cannot be read is not an empty folder.
func TestListProfileTemplatesNeverReportsAnUnreadableFolderAsEmpty(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions do not bind the superuser")
	}
	for name, mode := range map[string]os.FileMode{
		"cannot be opened":           0o000,
		"can be listed but not read": 0o400,
	} {
		t.Run(name, func(t *testing.T) {
			resource := t.TempDir()
			projects := filepath.Join(resource, "ProjectTemplates")
			writeTemplate(t, projects, "Band Session.RPP")
			if err := os.Chmod(projects, mode); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(projects, 0o700) })
			templates, available, truncated := listIn(t, resource)
			if !available || !truncated || len(templates) != 0 {
				t.Fatalf("templates = %v available = %v truncated = %v", templateNames(templates), available, truncated)
			}
		})
	}
}

func TestListProfileTemplatesIsCappedAndSaysSo(t *testing.T) {
	resource := t.TempDir()
	for i := 0; i < maxProfileTemplates; i++ {
		writeTemplate(t, filepath.Join(resource, "ProjectTemplates"), fmt.Sprintf("P%03d.RPP", i))
	}
	writeTemplate(t, filepath.Join(resource, "TrackTemplates"), "Extra.RTrackTemplate")
	templates, _, truncated := listIn(t, resource)
	if len(templates) != maxProfileTemplates || !truncated {
		t.Fatalf("%d templates, truncated = %v", len(templates), truncated)
	}
	// Exactly the limit is a complete list.
	if err := os.Remove(filepath.Join(resource, "TrackTemplates", "Extra.RTrackTemplate")); err != nil {
		t.Fatal(err)
	}
	if templates, _, truncated = listIn(t, resource); len(templates) != maxProfileTemplates || truncated {
		t.Fatalf("%d templates, truncated = %v", len(templates), truncated)
	}
}

// Only so much of a folder is looked at; a folder with more entries than that
// is reported as not fully listed even when nothing looked at was a template.
func TestListProfileTemplatesBoundsHowMuchOfAFolderItReads(t *testing.T) {
	resource := t.TempDir()
	projects := filepath.Join(resource, "ProjectTemplates")
	if err := os.MkdirAll(projects, 0o700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= maxProfileDirEntries; i++ {
		if err := os.WriteFile(filepath.Join(projects, fmt.Sprintf("n%05d.txt", i)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	templates, available, truncated := listIn(t, resource)
	if !available || !truncated || len(templates) != 0 {
		t.Fatalf("%d templates available = %v truncated = %v", len(templates), available, truncated)
	}
	// One entry fewer is the whole folder.
	if err := os.Remove(filepath.Join(projects, "n00000.txt")); err != nil {
		t.Fatal(err)
	}
	if _, _, truncated = listIn(t, resource); truncated {
		t.Fatal("a folder read to its end is reported as cut short")
	}
}

func TestListProfileTemplatesWithoutATemplateFolder(t *testing.T) {
	resource := t.TempDir()
	if templates, available, truncated := listIn(t, resource); templates != nil || available || truncated {
		t.Fatalf("no folders: %v %v %v", templates, available, truncated)
	}
	if templates, available, _ := listProfileTemplates(nil); templates != nil || available || profileTemplatesAvailable(nil) {
		t.Fatal("an unknown resource folder listed something")
	}
	// A template folder that is a link is not followed.
	elsewhere := t.TempDir()
	writeTemplate(t, elsewhere, "Elsewhere.RPP")
	if err := os.Symlink(elsewhere, filepath.Join(resource, "ProjectTemplates")); err != nil {
		t.Fatal(err)
	}
	if templates, available, truncated := listIn(t, resource); templates != nil || available || truncated {
		t.Fatalf("a linked template folder was followed: %v", templateNames(templates))
	}
	// An empty real folder is available and lists nothing.
	if err := os.Mkdir(filepath.Join(resource, "TrackTemplates"), 0o700); err != nil {
		t.Fatal(err)
	}
	if templates, available, truncated := listIn(t, resource); len(templates) != 0 || !available || truncated {
		t.Fatalf("empty folder: %v %v %v", templates, available, truncated)
	}
}

// A directory is used only when the one opened is the very one looked at, so
// a link slipped in between the two is refused.
func TestOpenRealDirOpensOnlyARealDirectory(t *testing.T) {
	parent := t.TempDir()
	root, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := os.Mkdir(filepath.Join(parent, "real"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(parent, "inside-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(parent, "outside-link")); err != nil {
		t.Fatal(err)
	}
	dir, exists, ok := openRealDir(root, "real")
	if !exists || !ok || dir == nil {
		t.Fatalf("a real directory: exists = %v ok = %v", exists, ok)
	}
	_ = dir.Close()
	for _, name := range []string{"missing", "file", "inside-link", "outside-link", "..", "real/.."} {
		if dir, exists, ok := openRealDir(root, name); dir != nil || exists || ok {
			t.Errorf("%s: opened = %v exists = %v ok = %v", name, dir != nil, exists, ok)
		}
	}
	if dir, exists, ok := openRealDir(nil, "real"); dir != nil || exists || ok {
		t.Fatal("a directory was opened without a parent")
	}
	if !regularFileIn(root, "file") || regularFileIn(root, "real") || regularFileIn(root, "inside-link") ||
		regularFileIn(root, "missing") || regularFileIn(nil, "file") {
		t.Fatal("regularFileIn misjudged an entry")
	}
}

const bundleInfoFixture = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleDocumentTypes</key>
	<array>
		<dict>
			<key>CFBundleTypeName</key>
			<string>ReaperProject</string>
			<key>LSTypeIsPackage</key>
			<false/>
		</dict>
	</array>
	<key>CFBundleName</key>
	<string>REAPER</string>
	<key>CFBundleShortVersionString</key>
	<string>%s</string>
	<key>CFBundleVersion</key>
	<string>9.99.0_build</string>
</dict>
</plist>
`

func TestParseBundleVersionReadsTheReleaseNumber(t *testing.T) {
	for raw, want := range map[string]string{
		"7.28.0_1a2b3c4d": "7.28",
		"7.08":            "7.08",
		"6.83+dev0412":    "6.83",
		"  7.28  ":        "7.28",
		"nightly":         "nightly",
	} {
		if got := parseBundleVersion([]byte(fmt.Sprintf(bundleInfoFixture, raw))); got != want {
			t.Errorf("version %q = %q, want %q", raw, got, want)
		}
	}
	// Without a short version the build version is used.
	onlyBuild := strings.Replace(fmt.Sprintf(bundleInfoFixture, "x"), "<key>CFBundleShortVersionString</key>\n\t<string>x</string>\n", "", 1)
	if got := parseBundleVersion([]byte(onlyBuild)); got != "9.99" {
		t.Errorf("build version fallback = %q", got)
	}
	// Only the list's own dictionary counts: a key of the same name nested in
	// a later dictionary does not replace it, and neither does a second one.
	nested := `<plist><dict><key>CFBundleShortVersionString</key><string>7.28</string>` +
		`<key>Extras</key><dict><key>CFBundleShortVersionString</key><string>1.0</string></dict>` +
		`<key>CFBundleShortVersionString</key><string>2.0</string></dict></plist>`
	if got := parseBundleVersion([]byte(nested)); got != "7.28" {
		t.Errorf("nested and repeated keys = %q, want the top-level first value", got)
	}
	// Anything else is no version, never an error.
	for name, data := range map[string][]byte{
		"empty":       nil,
		"binary list": []byte("bplist00\x00\x01\x02"),
		"not a list":  []byte("REAPER 7.28"),
		"damaged":     []byte("<plist><dict><key>CFBundleShortVersionString</key><string>"),
		"too large":   []byte(strings.Repeat("x", maxBundleInfoBytes+1)),
		"no version":  []byte("<plist><dict><key>CFBundleName</key><string>REAPER</string></dict></plist>"),
		"multi-line":  []byte(fmt.Sprintf(bundleInfoFixture, "line one\nline two")),
		"over long":   []byte(fmt.Sprintf(bundleInfoFixture, strings.Repeat("v", maxProfileVersionLength+1))),
		"an entity":   []byte("<plist><dict><key>CFBundleShortVersionString</key><string>&b;</string></dict></plist>"),
		"markup-like": []byte(fmt.Sprintf(bundleInfoFixture, "&lt;b&gt;")),
		"a real value under another key": []byte("<plist><dict><key>CFBundleShortVersionString</key><true/>" +
			"<key>Other</key><string>7.28</string></dict></plist>"),
		"a value inside an array after the key": []byte("<plist><dict><key>CFBundleShortVersionString</key>" +
			"<array><string>7.28</string></array></dict></plist>"),
		"only a nested dictionary has the key": []byte("<plist><dict><key>Extras</key><dict>" +
			"<key>CFBundleShortVersionString</key><string>7.28</string></dict></dict></plist>"),
	} {
		if got := parseBundleVersion(data); got != "" {
			t.Errorf("%s: version = %q, want none", name, got)
		}
	}
}

func TestServiceProfileBoundsWhateverThePlatformReports(t *testing.T) {
	listed := []ProfileTemplate{
		{Name: "Band Session", Kind: ProfileTemplateProject, File: "Band Session.RPP", ModifiedAt: "2026-10-01T10:00:00Z"},
		{Name: "Drum Bus", Kind: ProfileTemplateTrack, File: "Drum Bus.RTrackTemplate"},
	}
	probe := &profileProbeStub{result: ProfileResult{
		App: "something else", Installed: true, Version: "7.28.0_1a2b3c4d", TemplatesAvailable: true, Templates: listed,
	}}
	service := &Service{profile: probe}

	// Installation and version only: no template leaves the service, even if
	// the platform reported some.
	facts := service.Profile(context.Background(), ProfileInput{})
	if facts.App != "REAPER" || !facts.Installed || facts.Version != "7.28" || !facts.TemplatesAvailable ||
		facts.Templates != nil || facts.Truncated {
		t.Fatalf("version-only facts = %+v", facts)
	}
	if len(probe.asked) != 1 || probe.asked[0] {
		t.Fatalf("the platform was asked for templates: %v", probe.asked)
	}

	facts = service.Profile(context.Background(), ProfileInput{IncludeTemplates: true})
	if len(facts.Templates) != 2 || facts.Truncated || facts.Templates[0].ModifiedAt != "2026-10-01T10:00:00Z" {
		t.Fatalf("facts with templates = %+v", facts)
	}
	encoded, err := json.Marshal(facts)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &keys); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"app", "installed", "version", "templates_available", "templates", "truncated"} {
		if _, ok := keys[key]; !ok {
			t.Errorf("facts lack %q: %s", key, encoded)
		}
	}
	if len(keys) != 6 {
		t.Fatalf("facts carry a key the host does not know: %s", encoded)
	}

	// Entries a host could not accept are dropped and the list marked
	// incomplete; the list never exceeds the limit.
	probe.result.Templates = append([]ProfileTemplate{
		{Name: "Path", Kind: ProfileTemplateProject, File: "../Secret.RPP"},
		{Name: "Absolute", Kind: ProfileTemplateProject, File: "/Users/me/Secret.RPP"},
		{Name: "Preset", Kind: "preset", File: "Preset.RPP"},
		{Name: "", Kind: ProfileTemplateTrack, File: "Blank.RTrackTemplate"},
		{Name: "Back" + string(rune(0x202e)) + "wards", Kind: ProfileTemplateProject, File: "Backwards.RPP"},
		{Name: "Two" + string(rune(0x2028)) + "lines", Kind: ProfileTemplateProject, File: "Lines.RPP"},
	}, listed...)
	facts = service.Profile(context.Background(), ProfileInput{IncludeTemplates: true})
	if len(facts.Templates) != 2 || !facts.Truncated {
		t.Fatalf("facts with unusable entries = %+v", facts)
	}
	many := make([]ProfileTemplate, 0, maxProfileTemplates+5)
	for i := 0; i < maxProfileTemplates+5; i++ {
		name := fmt.Sprintf("T%03d", i)
		many = append(many, ProfileTemplate{Name: name, Kind: ProfileTemplateProject, File: name + ".RPP"})
	}
	probe.result.Templates = many
	facts = service.Profile(context.Background(), ProfileInput{IncludeTemplates: true})
	if len(facts.Templates) != maxProfileTemplates || !facts.Truncated {
		t.Fatalf("%d templates, truncated = %v", len(facts.Templates), facts.Truncated)
	}

	// A time that is not one is left out; the template is kept.
	probe.result.Templates = []ProfileTemplate{
		{Name: "Odd", Kind: ProfileTemplateProject, File: "Odd.RPP", ModifiedAt: strings.Repeat("9", 200)},
		{Name: "Words", Kind: ProfileTemplateProject, File: "Words.RPP", ModifiedAt: "yesterday"},
	}
	facts = service.Profile(context.Background(), ProfileInput{IncludeTemplates: true})
	if len(facts.Templates) != 2 || facts.Truncated || facts.Templates[0].ModifiedAt != "" || facts.Templates[1].ModifiedAt != "" {
		t.Fatalf("facts with odd times = %+v", facts)
	}
	// A name in a script that needs joining marks is an ordinary name.
	probe.result.Templates = []ProfileTemplate{
		{Name: "می" + string(rune(0x200c)) + "خواهم", Kind: ProfileTemplateProject, File: "Persian.RPP"},
		{Name: "밴드 세션 🎸", Kind: ProfileTemplateTrack, File: "밴드 세션.RTrackTemplate"},
	}
	if facts = service.Profile(context.Background(), ProfileInput{IncludeTemplates: true}); len(facts.Templates) != 2 || facts.Truncated {
		t.Fatalf("ordinary non-Latin names were refused: %+v", facts)
	}
}

// A host refuses an answer larger than the declared limit whole, and would
// then show nothing. However long the names are, and whatever they are made
// of, the answer is cut to fit and says so.
func TestServiceProfileAlwaysFitsTheDeclaredOutputSize(t *testing.T) {
	longest := func(character string, extension string) []ProfileTemplate {
		templates := make([]ProfileTemplate, 0, maxProfileTemplates)
		for i := 0; i < maxProfileTemplates; i++ {
			name := fmt.Sprintf("%03d", i) + strings.Repeat(character, maxProfileTemplateName-3)
			templates = append(templates, ProfileTemplate{
				Name: name, Kind: ProfileTemplateProject, File: name + extension, ModifiedAt: "2026-10-01T10:00:00Z",
			})
		}
		return templates
	}
	tests := map[string]struct {
		templates []ProfileTemplate
		complete  bool
	}{
		"plain letters":                 {longest("n", ".RPP"), true},
		"three-byte characters":         {longest("한", ".RPP"), false},
		"four-byte characters":          {longest("🎸", ".RPP"), false},
		"characters a host writes long": {longest("&", ".RPP"), false},
		"angle brackets":                {longest("<", ".RPP"), false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			probe := &profileProbeStub{result: ProfileResult{
				Installed: true, Version: strings.Repeat("9", maxProfileVersionLength), TemplatesAvailable: true, Templates: tc.templates,
			}}
			facts := (&Service{profile: probe}).Profile(context.Background(), ProfileInput{IncludeTemplates: true})
			// Counted the way a host counts: decoded and encoded again.
			encoded, err := json.Marshal(facts)
			if err != nil {
				t.Fatal(err)
			}
			var generic any
			if err := json.Unmarshal(encoded, &generic); err != nil {
				t.Fatal(err)
			}
			reencoded, err := json.Marshal(generic)
			if err != nil {
				t.Fatal(err)
			}
			if len(reencoded) > maxProfileOutputBytes || len(encoded) > maxProfileOutputBytes {
				t.Fatalf("the answer is %d bytes (%d re-encoded), over the declared %d", len(encoded), len(reencoded), maxProfileOutputBytes)
			}
			if complete := len(facts.Templates) == maxProfileTemplates && !facts.Truncated; complete != tc.complete {
				t.Fatalf("%d templates, truncated = %v, want complete = %v", len(facts.Templates), facts.Truncated, tc.complete)
			}
			if len(facts.Templates) == 0 {
				t.Fatal("nothing was listed")
			}
			if len(facts.Templates) < maxProfileTemplates && !facts.Truncated {
				t.Fatal("the list was cut without saying so")
			}
		})
	}
}

// The service built for the running plugin answers from the platform's reader.
// Without that hand-over every computer would read as having no REAPER.
func TestNewServiceAnswersTheProfileReadFromThePlatformProbe(t *testing.T) {
	probe := &profileProbeStub{result: ProfileResult{Installed: true, Version: "7.28"}}
	service := NewService(&Manager{}, ProbeSet{Profile: probe}, nil)
	if facts := service.Profile(context.Background(), ProfileInput{}); !facts.Installed || facts.Version != "7.28" {
		t.Fatalf("facts = %+v", facts)
	}
	if NewPlatformProbeSet(nil).Profile == nil {
		t.Fatal("the platform probe set has no profile reader")
	}
}

func TestServiceProfileSaysNothingAboutAnApplicationThatIsNotInstalled(t *testing.T) {
	probe := &profileProbeStub{result: ProfileResult{
		Version: "7.28", TemplatesAvailable: true,
		Templates: []ProfileTemplate{{Name: "Band Session", Kind: ProfileTemplateProject, File: "Band Session.RPP"}},
	}}
	facts := (&Service{profile: probe}).Profile(context.Background(), ProfileInput{IncludeTemplates: true})
	if facts.App != "REAPER" || facts.Installed || facts.Version != "" || facts.TemplatesAvailable || facts.Templates != nil || facts.Truncated {
		t.Fatalf("facts = %+v", facts)
	}
	// A service without a platform reader answers the same way.
	for _, service := range []*Service{nil, {}} {
		if facts := service.Profile(context.Background(), ProfileInput{IncludeTemplates: true}); facts.App != "REAPER" || facts.Installed || facts.Templates != nil {
			t.Fatalf("facts = %+v", facts)
		}
	}
	// Installed, with no template folder: nothing is listed and nothing is
	// reported as cut short.
	probe.result = ProfileResult{Installed: true, Truncated: true,
		Templates: []ProfileTemplate{{Name: "Stray", Kind: ProfileTemplateProject, File: "Stray.RPP"}}}
	if facts := (&Service{profile: probe}).Profile(context.Background(), ProfileInput{IncludeTemplates: true}); !facts.Installed || facts.Templates != nil || facts.Truncated {
		t.Fatalf("facts = %+v", facts)
	}
}

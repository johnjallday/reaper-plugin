package reaper

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProjectFile(t *testing.T, dir, name, content string) (string, int64) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, int64(len(content))
}

func projectFactsRequest(names bool, files ...ProjectFactsFileSpec) ProjectFactsInput {
	return ProjectFactsInput{SchemaVersion: 1, Names: names, MaxFileBytes: maxProjectFactsFileBytes, Files: files}
}

// The operation uses no client, runner, profile or Web Remote: it answers on a
// nil service, with nothing configured.
func readProjectFiles(t *testing.T, input ProjectFactsInput) ProjectFactsResult {
	t.Helper()
	result, err := (*Service)(nil).ProjectFacts(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestProjectFactsOperationAnswersOneEntryPerID(t *testing.T) {
	dir := t.TempDir()
	song, size := writeProjectFile(t, dir, "Song.rpp", realShapeProject)
	notProject, notSize := writeProjectFile(t, dir, "notes.rpp", "hello\n")
	result := readProjectFiles(t, projectFactsRequest(true,
		ProjectFactsFileSpec{ID: 3, Format: "reaper", Path: song, Size: size},
		ProjectFactsFileSpec{ID: 0, Format: "reaper", Path: notProject, Size: notSize},
		ProjectFactsFileSpec{ID: 7, Format: "reaper", Path: filepath.Join(dir, "missing.rpp"), Size: 0},
	))
	if result.SchemaVersion != 1 || len(result.Files) != 3 {
		t.Fatalf("result = %+v", result)
	}
	ids := []int{result.Files[0].ID, result.Files[1].ID, result.Files[2].ID}
	results := []string{result.Files[0].Result, result.Files[1].Result, result.Files[2].Result}
	if fmt.Sprint(ids) != "[3 0 7]" || fmt.Sprint(results) != "[read unreadable failed]" {
		t.Fatalf("ids = %v, results = %v", ids, results)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema_version":1,"files":[` +
		`{"id":3,"result":"read","tempo_bpm":120,"tempo_varies":false,"length_seconds":215.5,"track_count":2,` +
		`"tracks":["Kick","Snare Top"],"tracks_left_out":0,` +
		`"markers":[{"name":"Verse","at":10},{"name":"Chorus A","at":40,"end":72.25}],"markers_left_out":0},` +
		`{"id":0,"result":"unreadable"},{"id":7,"result":"failed"}]}`
	if string(encoded) != want {
		t.Fatalf("encoded =\n %s\nwant\n %s", encoded, want)
	}
}

func TestProjectFactsOperationFailsWhatItMustNotRead(t *testing.T) {
	dir := t.TempDir()
	song, size := writeProjectFile(t, dir, "Song.rpp", realShapeProject)
	link := filepath.Join(dir, "Link.rpp")
	if err := os.Symlink(song, link); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(dir, "Folder.rpp")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(dir, "Pipe.rpp")
	makeFIFO(t, fifo)
	relative, err := filepath.Rel(mustGetwd(t), song)
	if err != nil {
		t.Fatal(err)
	}
	cases := []ProjectFactsFileSpec{
		{ID: 0, Format: "reaper", Path: link, Size: size},
		{ID: 1, Format: "reaper", Path: folder, Size: 0},
		{ID: 2, Format: "reaper", Path: song, Size: size + 1},
		{ID: 3, Format: "reaper", Path: relative, Size: size},
		{ID: 4, Format: "ableton", Path: song, Size: size},
		{ID: 5, Format: "reaper", Path: fifo, Size: 0},
		{ID: 6, Format: "reaper", Path: "", Size: size},
		{ID: 7, Format: "reaper", Path: song, Size: size},
	}
	result := readProjectFiles(t, projectFactsRequest(true, cases...))
	for i, entry := range result.Files {
		want := ProjectFactsFailed
		if i == 7 {
			want = ProjectFactsRead
		}
		if entry.ID != i || entry.Result != want {
			t.Errorf("case %d (%s) = %+v, want %s", i, cases[i].Path, entry, want)
		}
		if want == ProjectFactsFailed {
			encoded, _ := json.Marshal(entry)
			if string(encoded) != fmt.Sprintf(`{"id":%d,"result":"failed"}`, i) {
				t.Errorf("a failed entry carries more than id and result: %s", encoded)
			}
		}
	}
	// The read wrote nothing next to the song.
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 4 {
		t.Fatalf("directory now holds %d entries, %v", len(entries), err)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}

func TestProjectFactsOperationWithoutNamesSendsNoNameField(t *testing.T) {
	dir := t.TempDir()
	song, size := writeProjectFile(t, dir, "Song.rpp", realShapeProject)
	result := readProjectFiles(t, projectFactsRequest(false, ProjectFactsFileSpec{ID: 0, Format: "reaper", Path: song, Size: size}))
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema_version":1,"files":[{"id":0,"result":"read","tempo_bpm":120,"tempo_varies":false,"length_seconds":215.5,"track_count":2}]}`
	if string(encoded) != want {
		t.Fatalf("encoded = %s", encoded)
	}
	for _, name := range []string{"Kick", "Snare", "Verse", "Chorus"} {
		if strings.Contains(string(encoded), name) {
			t.Fatalf("a numbers-only answer named %q", name)
		}
	}
	// Too large and unreadable answers carry only id and result too.
	result = readProjectFiles(t, ProjectFactsInput{SchemaVersion: 1, Names: true, MaxFileBytes: 16, Files: []ProjectFactsFileSpec{{ID: 0, Format: "reaper", Path: song, Size: size}}})
	if encoded, _ = json.Marshal(result.Files[0]); string(encoded) != `{"id":0,"result":"too_large"}` {
		t.Fatalf("too large = %s", encoded)
	}
}

// worstCaseProject reaches every cap with the longest encoding each field can
// take: 128 track names and 128 regions whose names spend the whole byte cap
// on escaped characters, the largest times and left-out counts.
func worstCaseProject() string {
	var b strings.Builder
	b.WriteString("<REAPER_PROJECT\nTEMPO 123.45\n")
	name := strings.Repeat("&", maxProjectFactsNameCost/maxProjectFactsNames/6) // cost 96 each
	for i := range maxProjectFactsNames + 200 {
		fmt.Fprintf(&b, "<TRACK\nNAME %s\n<ITEM\nPOSITION 359000.123\nLENGTH 999.876\n>\n>\n", name)
		fmt.Fprintf(&b, "MARKER %d 358%03d.123 %s 1\nMARKER %d 359%03d.987 \"\" 1\n", i, i%1000, name, i, i%1000)
	}
	b.WriteString(">\n")
	return b.String()
}

func TestProjectFactsOperationEightFilesAtEveryCapFitTheOutputLimit(t *testing.T) {
	dir := t.TempDir()
	content := worstCaseProject()
	files := make([]ProjectFactsFileSpec, 0, maxProjectFactsFiles)
	for id := range maxProjectFactsFiles {
		path, size := writeProjectFile(t, dir, fmt.Sprintf("Song %d.rpp", id), content)
		files = append(files, ProjectFactsFileSpec{ID: id, Format: "reaper", Path: path, Size: size})
	}
	result := readProjectFiles(t, projectFactsRequest(true, files...))
	for _, entry := range result.Files {
		if entry.Result != ProjectFactsRead || len(entry.Tracks) != maxProjectFactsNames || len(entry.Markers) != maxProjectFactsNames ||
			*entry.TracksLeftOut != 200 || *entry.MarkersLeftOut != 200 || entry.Markers[0].End == 0 {
			t.Fatalf("entry %d did not reach the caps: %s, %d tracks, %d markers", entry.ID, entry.Result, len(entry.Tracks), len(entry.Markers))
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > maxProjectFactsOutputBytes {
		t.Fatalf("eight files at every cap encode to %d bytes, over %d", len(encoded), maxProjectFactsOutputBytes)
	}
	t.Logf("eight files at every cap encode to %d of %d bytes", len(encoded), maxProjectFactsOutputBytes)
	// The largest left-out counts add at most a few digits per file.
	if headroom := maxProjectFactsOutputBytes - len(encoded); headroom < maxProjectFactsFiles*2*4 {
		t.Fatalf("only %d bytes of headroom", headroom)
	}
}

func TestProjectFactsOperationRefusesAnInvalidRequest(t *testing.T) {
	file := ProjectFactsFileSpec{ID: 0, Format: "reaper", Path: "/a.rpp", Size: 1}
	nine := make([]ProjectFactsFileSpec, 9)
	for i := range nine {
		nine[i] = ProjectFactsFileSpec{ID: i % 8, Format: "reaper", Path: "/a.rpp"}
	}
	for name, input := range map[string]ProjectFactsInput{
		"schema version":  {SchemaVersion: 2, MaxFileBytes: 1, Files: []ProjectFactsFileSpec{file}},
		"no max":          {SchemaVersion: 1, MaxFileBytes: 0, Files: []ProjectFactsFileSpec{file}},
		"max too large":   {SchemaVersion: 1, MaxFileBytes: maxProjectFactsFileBytes + 1, Files: []ProjectFactsFileSpec{file}},
		"no files":        {SchemaVersion: 1, MaxFileBytes: 1},
		"nine files":      {SchemaVersion: 1, MaxFileBytes: 1, Files: nine},
		"duplicate id":    {SchemaVersion: 1, MaxFileBytes: 1, Files: []ProjectFactsFileSpec{file, file}},
		"id out of range": {SchemaVersion: 1, MaxFileBytes: 1, Files: []ProjectFactsFileSpec{{ID: 8, Format: "reaper", Path: "/a"}}},
		"negative id":     {SchemaVersion: 1, MaxFileBytes: 1, Files: []ProjectFactsFileSpec{{ID: -1, Format: "reaper", Path: "/a"}}},
		"long format":     {SchemaVersion: 1, MaxFileBytes: 1, Files: []ProjectFactsFileSpec{{Format: strings.Repeat("r", 65), Path: "/a"}}},
		"long path":       {SchemaVersion: 1, MaxFileBytes: 1, Files: []ProjectFactsFileSpec{{Format: "reaper", Path: "/" + strings.Repeat("a", 4096)}}},
		"negative size":   {SchemaVersion: 1, MaxFileBytes: 1, Files: []ProjectFactsFileSpec{{Format: "reaper", Path: "/a", Size: -1}}},
	} {
		if _, err := (*Service)(nil).ProjectFacts(context.Background(), input); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// The bounds themselves are accepted.
	edge := ProjectFactsInput{SchemaVersion: 1, MaxFileBytes: maxProjectFactsFileBytes, Files: []ProjectFactsFileSpec{
		{ID: 7, Format: strings.Repeat("r", 64), Path: "/" + strings.Repeat("é", 4095), Size: 0},
	}}
	if result, err := (*Service)(nil).ProjectFacts(context.Background(), edge); err != nil || result.Files[0].Result != ProjectFactsFailed {
		t.Fatalf("edge request = %+v, %v", result, err)
	}
}

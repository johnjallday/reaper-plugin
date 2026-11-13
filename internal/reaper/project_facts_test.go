package reaper

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func readProject(t *testing.T, content string, names bool) (string, ProjectFacts, *ProjectNames) {
	t.Helper()
	return ReadProjectFacts(strings.NewReader(content), maxProjectFactsFileBytes, names)
}

func wantRead(t *testing.T, content string, want ProjectFacts) {
	t.Helper()
	result, facts, names := readProject(t, content, false)
	if result != ProjectFactsRead || names != nil {
		t.Fatalf("result = %s, names = %+v", result, names)
	}
	if facts != want {
		t.Fatalf("facts = %+v, want %+v", facts, want)
	}
}

func wantNames(t *testing.T, content string, want ProjectNames) {
	t.Helper()
	result, _, names := readProject(t, content, true)
	if result != ProjectFactsRead || names == nil {
		t.Fatalf("result = %s, names = %+v", result, names)
	}
	if !reflect.DeepEqual(*names, want) {
		got, _ := json.Marshal(names)
		expected, _ := json.Marshal(want)
		t.Fatalf("names =\n %s\nwant\n %s", got, expected)
	}
}

func wantResult(t *testing.T, content string, want string) {
	t.Helper()
	for _, names := range []bool{false, true} {
		if result, _, _ := readProject(t, content, names); result != want {
			t.Fatalf("names=%t: result = %s, want %s", names, result, want)
		}
	}
}

const realShapeProject = `<REAPER_PROJECT 0.1 "7.28/macOS-arm64" 1730000000
  RIPPLE 0
  TEMPO 120.04 4 4
  MARKER 1 10 Verse 0
  MARKER 2 40 "Chorus A" 1
  MARKER 2 72.25 "" 1
  <PROJBAY
  >
  <TEMPOENVEX
    EGUID {00000000-0000-0000-0000-000000000000}
    PT 0 120.04 0
    PT 30 120.0 0
  >
  <TRACK {A}
    NAME Kick
    <FXCHAIN
      NAME "not a track name"
    >
    <ITEM
      POSITION 2
      LENGTH 10.5
      NAME "an item name"
    >
    <ITEM
      POSITION 200
      LENGTH 15.5
    >
  >
  <TRACK {B}
    NAME 'Snare Top'
  >
>
`

func TestProjectFactsReadsTheThreeNumbers(t *testing.T) {
	wantRead(t, realShapeProject, ProjectFacts{TempoBPM: 120, TempoVaries: false, LengthSeconds: 215.5, TrackCount: 2})
	// Tempo rounds to one decimal, length to three.
	wantRead(t, "<REAPER_PROJECT\nTEMPO 99.96\n<TRACK\n<ITEM\nPOSITION 1.00049\nLENGTH 1\n>\n>\n>\n",
		ProjectFacts{TempoBPM: 100, LengthSeconds: 2, TrackCount: 1})
	// A bad tempo line is skipped and a later good one taken; only the first
	// good one counts; a tempo out of 1 to 960 is not a tempo.
	wantRead(t, "<REAPER_PROJECT\nTEMPO abc\nTEMPO 961\nTEMPO 0.5\nTEMPO 90\nTEMPO 100\n>\n", ProjectFacts{TempoBPM: 90})
	// Unknown tempo, no items, no tracks.
	wantRead(t, "<REAPER_PROJECT\n>\n", ProjectFacts{})
	// Numbers: negative, infinite, NaN and over-long values are not numbers;
	// a tempo attribute must be followed by a space or a tab.
	wantRead(t, "<REAPER_PROJECT\nTEMPO -120\nTEMPO inf\nTEMPO NaN\nTEMPO 000000000000000000000000000000120\nTEMPOX 80\n>\n", ProjectFacts{})
	wantRead(t, "<REAPER_PROJECT\nTEMPO\t1e2\n>\n", ProjectFacts{TempoBPM: 100})
	// An item needs both a known start and a known length; a length over
	// 360000 is unknown.
	wantRead(t, "<REAPER_PROJECT\n<TRACK\n<ITEM\nLENGTH 5\n>\n<ITEM\nPOSITION 3\nLENGTH x\n>\n>\n>\n", ProjectFacts{TrackCount: 1})
	wantRead(t, "<REAPER_PROJECT\n<TRACK\n<ITEM\nPOSITION 359999\nLENGTH 2\n>\n>\n>\n", ProjectFacts{TrackCount: 1})
	// Tracks, items and tempo blocks count only at their own level.
	wantRead(t, "<REAPER_PROJECT\n<FOLDER\n<TRACK\n<ITEM\nPOSITION 1\nLENGTH 1\n>\n>\n>\n<ITEM\nPOSITION 1\nLENGTH 9\n>\n<TRACK\n<TRACK\n>\n<TEMPOENVEX\nPT 0 200\n>\n>\nTEMPO 100\n>\n",
		ProjectFacts{TempoBPM: 100, TrackCount: 1})
}

func TestProjectFactsTempoVaries(t *testing.T) {
	// Points equal to the base tempo, after rounding, do not make it vary.
	wantRead(t, "<REAPER_PROJECT\nTEMPO 120\n<TEMPOENVEX\nPT 0 120\nPT 10 120.04\n>\n>\n", ProjectFacts{TempoBPM: 120})
	wantRead(t, "<REAPER_PROJECT\nTEMPO 120\n<TEMPOENVEX\nPT 0 120\nPT 10 121\n>\n>\n", ProjectFacts{TempoBPM: 120, TempoVaries: true})
	wantRead(t, "<REAPER_PROJECT\nTEMPO 120\n<TEMPOENVEX\nPT 10 120.06\n>\n>\n", ProjectFacts{TempoBPM: 120, TempoVaries: true})
	// Without a base tempo, points decide nothing; bad points are ignored.
	wantRead(t, "<REAPER_PROJECT\n<TEMPOENVEX\nPT 0 90\nPT 1 140\n>\n>\n", ProjectFacts{})
	wantRead(t, "<REAPER_PROJECT\nTEMPO 120\n<TEMPOENVEX\nPT 0\nPT 0 2000\nPT 0 -1\n>\n>\n", ProjectFacts{TempoBPM: 120})
}

func TestProjectFactsTrackBound(t *testing.T) {
	project := func(tracks int) string {
		return "<REAPER_PROJECT\n" + strings.Repeat("<TRACK\n>\n", tracks) + ">\n"
	}
	wantRead(t, project(maxProjectFactsTracks), ProjectFacts{TrackCount: maxProjectFactsTracks})
	wantRead(t, project(maxProjectFactsTracks+1), ProjectFacts{})
}

func TestProjectFactsRefusals(t *testing.T) {
	deep := func(levels int) string {
		return "<REAPER_PROJECT\n" + strings.Repeat("<X\n", levels-1) + strings.Repeat(">\n", levels)
	}
	wantResult(t, deep(maxProjectFactsDepth), ProjectFactsRead)
	for name, content := range map[string]string{
		"binary":              "<REAPER_PROJECT\nNAME a\x00b\n>\n",
		"binary later":        "<REAPER_PROJECT\n>\n\x00",
		"wrong first line":    "<TRACK\n>\n",
		"root prefix only":    "<REAPER_PROJECTS\n>\n",
		"text before root":    "hello\n<REAPER_PROJECT\n>\n",
		"content after close": "<REAPER_PROJECT\n>\nTEMPO 120\n",
		"block after close":   "<REAPER_PROJECT\n>\n<REAPER_PROJECT\n>\n",
		"too deep":            deep(maxProjectFactsDepth + 1),
		"empty":               "",
		"blank only":          "  \n\t\r\n\n",
		"not closed":          "<REAPER_PROJECT\n<TRACK\n>\n",
	} {
		t.Run(name, func(t *testing.T) { wantResult(t, content, ProjectFactsUnreadable) })
	}
	// Blank lines, CRLF and surrounding white space are not refusals.
	wantResult(t, "\r\n  <REAPER_PROJECT\t0.1\r\n\r\n   >  \r\n\n", ProjectFactsRead)
}

func TestProjectFactsOverCapIsNotUnreadable(t *testing.T) {
	long := "<REAPER_PROJECT\nNOTES " + strings.Repeat("x", maxProjectFactsLineBytes) + "\n>\n"
	wantResult(t, long, ProjectFactsTooLarge)
	small := "<REAPER_PROJECT\n" + strings.Repeat("<TRACK\n>\n", 10) + ">\n"
	if result, _, _ := ReadProjectFacts(strings.NewReader(small), int64(len(small)), false); result != ProjectFactsRead {
		t.Fatalf("a file of exactly max bytes = %s", result)
	}
	if result, _, _ := ReadProjectFacts(strings.NewReader(small), int64(len(small)-1), false); result != ProjectFactsTooLarge {
		t.Fatalf("a file one byte over max = %s", result)
	}
	// Trailing blank content still counts against the cap.
	padded := "<REAPER_PROJECT\n>\n" + strings.Repeat("\n", 64)
	if result, _, _ := ReadProjectFacts(strings.NewReader(padded), 32, false); result != ProjectFactsTooLarge {
		t.Fatalf("padded file over max = %s", result)
	}
	// A refusal met before the cap is reported as soon as it is met.
	if result, _, _ := ReadProjectFacts(strings.NewReader("<REAPER_PROJECT\n>\nX\n"+strings.Repeat("a", 100)), 32, false); result != ProjectFactsUnreadable {
		t.Fatalf("content after the close before the cap = %s", result)
	}
}

type failingReader struct {
	content io.Reader
}

func (f *failingReader) Read(p []byte) (int, error) {
	n, err := f.content.Read(p)
	if errors.Is(err, io.EOF) {
		return n, errors.New("device went away")
	}
	return n, err
}

func TestProjectFactsReadErrorIsFailed(t *testing.T) {
	for _, content := range []string{"<REAPER_PROJECT\n>\n", "<REAPER_PROJECT\n<TRACK\n", ""} {
		result, _, _ := ReadProjectFacts(&failingReader{strings.NewReader(content)}, maxProjectFactsFileBytes, true)
		if result != ProjectFactsFailed {
			t.Fatalf("%q with a read error = %s", content, result)
		}
	}
}

func TestProjectFactsNamesInProjectOrder(t *testing.T) {
	wantNames(t, realShapeProject, ProjectNames{
		Tracks:  []string{"Kick", "Snare Top"},
		Markers: []ProjectMarker{{Name: "Verse", At: 10}, {Name: "Chorus A", At: 40, End: 72.25}},
	})
	// Nothing to name: empty lists and zeros, never nil.
	result, _, names := readProject(t, "<REAPER_PROJECT\n>\n", true)
	if result != ProjectFactsRead || names == nil || names.Tracks == nil || names.Markers == nil {
		t.Fatalf("names = %+v", names)
	}
	encoded, _ := json.Marshal(names)
	if string(encoded) != `{"tracks":[],"tracks_left_out":0,"markers":[],"markers_left_out":0}` {
		t.Fatalf("empty names = %s", encoded)
	}
}

func TestProjectFactsOnlyATracksOwnFirstNameLineCounts(t *testing.T) {
	wantNames(t, `<REAPER_PROJECT
<TRACK
<ITEM
NAME "item"
>
NAME First
NAME Second
>
<TRACK
NAME "bad"quote
NAME Later
>
<TRACK
<FXCHAIN
NAME fx
>
>
NAME root
<OTHER
<TRACK
NAME nested
>
>
>
`, ProjectNames{Tracks: []string{"First"}, Markers: []ProjectMarker{}})
}

func TestProjectFactsQuoteStyles(t *testing.T) {
	wantNames(t, "<REAPER_PROJECT\n"+
		"<TRACK\nNAME \"Lead Vox\" 1\n>\n"+
		"<TRACK\nNAME 'Back Vox'\t2\n>\n"+
		"<TRACK\nNAME `Bass \"DI\"`\n>\n"+
		"<TRACK\nNAME Plain words\n>\n"+
		"<TRACK\nNAME \"never closes\n>\n"+
		"<TRACK\nNAME 'x'y\n>\n"+
		"<TRACK\nNAME ok \"bad\n>\n"+
		"<TRACK\nNAME It's\n>\n"+
		"MARKER 1 5 \"open 0\n"+
		"MARKER 2 6 'Bridge' 0 \"trailing\"x\n"+
		"MARKER 3 7 `Out` 0\n"+
		">\n", ProjectNames{
		Tracks:  []string{"Lead Vox", "Back Vox", `Bass "DI"`, "Plain", "It's"},
		Markers: []ProjectMarker{{Name: "Out", At: 7}},
	})
}

func TestProjectFactsEntryCapKeepsTheFirst(t *testing.T) {
	var b strings.Builder
	b.WriteString("<REAPER_PROJECT\n")
	for i := range maxProjectFactsNames + 5 {
		fmt.Fprintf(&b, "<TRACK\nNAME T%d\n>\nMARKER %d %d M%d 0\n", i, i, i, i)
	}
	b.WriteString(">\n")
	_, _, names := readProject(t, b.String(), true)
	if len(names.Tracks) != maxProjectFactsNames || names.Tracks[0] != "T0" || names.Tracks[127] != "T127" || names.TracksLeftOut != 5 {
		t.Fatalf("tracks = %d, first %q, last %q, left out %d", len(names.Tracks), names.Tracks[0], names.Tracks[len(names.Tracks)-1], names.TracksLeftOut)
	}
	if len(names.Markers) != maxProjectFactsNames || names.Markers[127].Name != "M127" || names.MarkersLeftOut != 5 {
		t.Fatalf("markers = %d, left out %d", len(names.Markers), names.MarkersLeftOut)
	}
}

func TestProjectFactsByteCapKeepsTheFirst(t *testing.T) {
	// 80 ampersands cost 480 bytes; 25 of them fit in 12288, the 26th does not.
	heavy := strings.Repeat("&", 80)
	var b strings.Builder
	b.WriteString("<REAPER_PROJECT\n")
	for i := range 30 {
		fmt.Fprintf(&b, "<TRACK\nNAME %s\n>\nMARKER %d %d %s 0\n", heavy, i, i, heavy)
	}
	// A name dropped by cleaning is not left out; a cheap name still fits.
	b.WriteString("<TRACK\nNAME /Users/me/a.wav\n>\n<TRACK\nNAME ok\n>\nMARKER 99 99 ok 0\n>\n")
	_, _, names := readProject(t, b.String(), true)
	if len(names.Tracks) != 26 || names.Tracks[25] != "ok" || names.TracksLeftOut != 5 {
		t.Fatalf("tracks = %d (last %q), left out %d", len(names.Tracks), names.Tracks[len(names.Tracks)-1], names.TracksLeftOut)
	}
	if len(names.Markers) != 26 || names.Markers[25].Name != "ok" || names.MarkersLeftOut != 5 {
		t.Fatalf("markers = %d, left out %d", len(names.Markers), names.MarkersLeftOut)
	}
	if songNameCost(`a"b\c<d>e&f`) != 1+2+1+2+1+6+1+6+1+6+1 {
		t.Fatalf("cost = %d", songNameCost(`a"b\c<d>e&f`))
	}
}

func TestProjectFactsRegionCutByTheCapCountedOnce(t *testing.T) {
	var b strings.Builder
	b.WriteString("<REAPER_PROJECT\n")
	for i := range maxProjectFactsNames {
		fmt.Fprintf(&b, "MARKER %d %d M 0\n", i+1000, i)
	}
	// A region start and its end past the cap: one left out, not two.
	b.WriteString("MARKER 7 200 Late 1\nMARKER 7 210 \"\" 1\n")
	// A plain marker past the cap.
	b.WriteString("MARKER 8 220 Later 0\n>\n")
	_, _, names := readProject(t, b.String(), true)
	if len(names.Markers) != maxProjectFactsNames || names.MarkersLeftOut != 2 {
		t.Fatalf("markers = %d, left out %d", len(names.Markers), names.MarkersLeftOut)
	}
}

func TestProjectFactsRegionPairing(t *testing.T) {
	wantNames(t, `<REAPER_PROJECT
MARKER 1 10 "At start" 1
MARKER 1 10 "" 1
MARKER 2 20 Backwards 1
MARKER 2 15 "" 1
MARKER 3 30 "No end" 1
MARKER 4 40 "Lonely end" 3
MARKER 5 50 Outer 1
MARKER 5 60 Second 1
MARKER 5 70 "" 1
MARKER 5 80 "" 1
MARKER 6 abc Bad 1
MARKER 6 90 "" 0
MARKER 9 1.0004 Rounded 0
MARKER 4294967296 1 TooBig 0
MARKER 10 360001 Late 0
MARKER 11 1 Flags x
MARKER 12 1 Short
>
`, ProjectNames{Tracks: []string{}, Markers: []ProjectMarker{
		{Name: "At start", At: 10},
		{Name: "Backwards", At: 20},
		{Name: "No end", At: 30},
		{Name: "Lonely end", At: 40},
		{Name: "Outer", At: 50, End: 60},
		// The second start line closed "Outer"; its name went with it. The
		// next region line starts a new, unnamed region.
		{At: 70, End: 80},
		{At: 90},
		{Name: "Rounded", At: 1},
	}})
}

func TestProjectFactsNameCleaning(t *testing.T) {
	for raw, want := range map[string]string{
		"  \t Lead":                    "Lead",
		"Lead   ":                      "Lead",
		"A\x01B\x7fC":                  "ABC",
		"bad\xffutf8":                  "badutf8",
		"bidi\u202Eflip\u2066x\u2069":  "bidiflipx",
		"line\u2028sep\u2029":          "linesep",
		"Vox L/R":                      "Vox L/R",
		"AC/DC":                        "AC/DC",
		"Señal ☃":                      "Señal ☃",
		"\ufffd kept":                  "\ufffd kept",
		strings.Repeat("é", 90):        strings.Repeat("é", 80),
		strings.Repeat("a", 79) + " b": strings.Repeat("a", 79),
	} {
		got, ok := songName([]byte(raw))
		if !ok || got != want {
			t.Errorf("songName(%q) = %q, %t; want %q", raw, got, ok, want)
		}
	}
	for _, raw := range []string{
		"", "   ", "\x01\x02", ".", "..", "~", "/Users/me/a.wav", "~/Music", "./take", "../take",
		"take\\1", "Mix /Volumes/x", "Bounce ~/a", "Stem (/tmp/a)", "C:/Audio", "c:\\Audio", "  /lead",
	} {
		if got, ok := songName([]byte(raw)); ok {
			t.Errorf("songName(%q) = %q, want dropped", raw, got)
		}
	}
	// A marker whose name is dropped keeps its time; a track does not.
	wantNames(t, "<REAPER_PROJECT\n<TRACK\nNAME /a/b\n>\n<TRACK\nNAME \"\"\n>\nMARKER 1 3 /a/b 0\nMARKER 2 4 \"\" 0\n>\n",
		ProjectNames{Tracks: []string{}, Markers: []ProjectMarker{{At: 3}, {At: 4}}})
}

// projectStream produces a large project without holding it in memory.
type projectStream struct {
	head, body, tail string
	repeats          int
	pending          string
	onRepeat         func(int)
}

func (s *projectStream) Read(p []byte) (int, error) {
	for s.pending == "" {
		switch {
		case s.head != "":
			s.pending, s.head = s.head, ""
		case s.repeats > 0:
			if s.onRepeat != nil {
				s.onRepeat(s.repeats)
			}
			s.pending = s.body
			s.repeats--
		case s.tail != "":
			s.pending, s.tail = s.tail, ""
		default:
			return 0, io.EOF
		}
	}
	n := copy(p, s.pending)
	s.pending = s.pending[n:]
	return n, nil
}

func TestProjectFactsMemoryDoesNotGrowWithTheFile(t *testing.T) {
	body := "<TRACK\nNAME \"A track with a name\"\n<ITEM\nPOSITION 1\nLENGTH 2\n>\n>\nMARKER 1 5 \"Region\" 1\nMARKER 1 6 \"\" 1\n<TEMPOENVEX\nPT 0 120\n>\n"
	const repeats = 200000 // about 22 MB
	var heap [2]uint64
	stream := &projectStream{
		head: "<REAPER_PROJECT\nTEMPO 120\n", body: body, tail: ">\n", repeats: repeats,
		onRepeat: func(left int) {
			if left == repeats-2000 || left == 1 {
				runtime.GC()
				var stats runtime.MemStats
				runtime.ReadMemStats(&stats)
				if left == 1 {
					heap[1] = stats.HeapAlloc
				} else {
					heap[0] = stats.HeapAlloc
				}
			}
		},
	}
	result, facts, names := ReadProjectFacts(stream, maxProjectFactsFileBytes, true)
	if result != ProjectFactsRead || facts.TrackCount != 0 || names.TracksLeftOut == 0 {
		t.Fatalf("result = %s, facts = %+v", result, facts)
	}
	if heap[0] == 0 || heap[1] == 0 {
		t.Fatalf("the heap was not measured: %v", heap)
	}
	if heap[1] > heap[0]+512*1024 {
		t.Fatalf("live heap grew from %d to %d bytes while reading", heap[0], heap[1])
	}
}

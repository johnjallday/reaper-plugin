package reaper

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The project facts reader reads what a host shows about a song from one
// saved project file: tempo, length, track count and, only when the person
// approved names, track names, markers and regions. It is a pure function over
// an io.Reader: it opens nothing, writes nothing and logs nothing. It holds one
// fixed stack of open blocks and one line at a time; the only things it keeps
// that depend on the content are the name lists, which are capped.
//
// For the same bytes it must answer exactly what the host's own reader
// answered before this operation existed, so the rules below follow that
// reader rule for rule, down to the standard-library calls.

// Results of reading one file.
const (
	ProjectFactsRead       = "read"
	ProjectFactsTooLarge   = "too_large"
	ProjectFactsUnreadable = "unreadable"
	ProjectFactsFailed     = "failed"
)

// Bounds of the reader. The output schema in the manifest declares the same
// numbers; TestProjectFactsBoundsAreTheOnesTheManifestDeclares holds them
// together.
const (
	maxProjectFactsLineBytes   = 262144
	maxProjectFactsDepth       = 64
	maxProjectFactsFileBytes   = 33554432
	maxProjectFactsTempo       = 960
	maxProjectFactsLength      = 360000
	maxProjectFactsTracks      = 10000
	maxProjectFactsNames       = 128
	maxProjectFactsNameRunes   = 80
	maxProjectFactsNameCost    = 12288
	maxProjectFactsLeftOut     = 1048576
	maxProjectFactsNumberBytes = 32
	maxProjectFactsMarkerID    = 20
)

// Keywords of this application's project format.
var (
	projectRootKeyword     = []byte("<REAPER_PROJECT")
	projectTempoKey        = []byte("TEMPO")
	projectTempoBlock      = []byte("TEMPOENVEX")
	projectTempoPointKey   = []byte("PT")
	projectTrackBlock      = []byte("TRACK")
	projectItemBlock       = []byte("ITEM")
	projectItemPositionKey = []byte("POSITION")
	projectItemLengthKey   = []byte("LENGTH")
	projectNameKey         = []byte("NAME")
	projectMarkerKey       = []byte("MARKER")
)

// ProjectFacts are the numbers read from one project.
type ProjectFacts struct {
	TempoBPM      float64 `json:"tempo_bpm"`
	TempoVaries   bool    `json:"tempo_varies"`
	LengthSeconds float64 `json:"length_seconds"`
	TrackCount    int     `json:"track_count"`
}

// ProjectNames are the names read from one project, each list capped.
type ProjectNames struct {
	Tracks         []string        `json:"tracks"`
	TracksLeftOut  int             `json:"tracks_left_out"`
	Markers        []ProjectMarker `json:"markers"`
	MarkersLeftOut int             `json:"markers_left_out"`
}

// ProjectMarker is a marker, or a region when End is set. A region's end is
// always after its start, which is never negative, so a zero End means none.
type ProjectMarker struct {
	Name string  `json:"name,omitempty"`
	At   float64 `json:"at"`
	End  float64 `json:"end,omitempty"`
}

type projectBlock uint8

const (
	projectBlockOther projectBlock = iota
	projectBlockRoot
	projectBlockTrack
	projectBlockItem
	projectBlockTempo
)

// ReadProjectFacts reads one project from r, consuming at most maxBytes+1
// bytes. Names are collected only when names is true; otherwise the returned
// names are nil. Facts and names are meaningful only when the result is
// ProjectFactsRead.
func ReadProjectFacts(r io.Reader, maxBytes int64, names bool) (string, ProjectFacts, *ProjectNames) {
	counted := &countingReader{r: io.LimitReader(r, maxBytes+1)}
	scanner := bufio.NewScanner(counted)
	scanner.Buffer(make([]byte, 0, 4096), maxProjectFactsLineBytes)

	var (
		stack       [maxProjectFactsDepth]projectBlock
		depth       int
		started     bool
		closed      bool
		baseTempo   float64
		pointSeen   bool
		pointMin    float64
		pointMax    float64
		tracks      int
		length      float64
		itemStart   float64
		itemLength  float64
		startKnown  bool
		lengthKnown bool
		trackNamed  bool
		collector   *nameCollector
	)
	if names {
		collector = &nameCollector{}
	}

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if bytes.IndexByte(line, 0) >= 0 {
			return ProjectFactsUnreadable, ProjectFacts{}, nil
		}
		if len(line) == 0 {
			continue
		}
		if closed {
			return ProjectFactsUnreadable, ProjectFacts{}, nil
		}
		if !started {
			if !bytes.HasPrefix(line, projectRootKeyword) {
				return ProjectFactsUnreadable, ProjectFacts{}, nil
			}
			if rest := line[len(projectRootKeyword):]; len(rest) > 0 && rest[0] != ' ' && rest[0] != '\t' {
				return ProjectFactsUnreadable, ProjectFacts{}, nil
			}
			started = true
		}
		if line[0] == '<' {
			if depth == maxProjectFactsDepth {
				return ProjectFactsUnreadable, ProjectFacts{}, nil
			}
			name := line[1:]
			if end := bytes.IndexAny(name, " \t"); end >= 0 {
				name = name[:end]
			}
			kind := projectBlockOther
			inner := projectBlockOther
			if depth > 0 {
				inner = stack[depth-1]
			}
			switch {
			case depth == 0:
				kind = projectBlockRoot
			case inner == projectBlockRoot && bytes.Equal(name, projectTrackBlock):
				kind = projectBlockTrack
				if tracks <= maxProjectFactsTracks {
					tracks++
				}
				trackNamed = false
			case inner == projectBlockTrack && bytes.Equal(name, projectItemBlock):
				kind = projectBlockItem
				startKnown, lengthKnown = false, false
			case inner == projectBlockRoot && bytes.Equal(name, projectTempoBlock):
				kind = projectBlockTempo
			}
			stack[depth] = kind
			depth++
			continue
		}
		if len(line) == 1 && line[0] == '>' {
			if depth > 0 {
				depth--
				if stack[depth] == projectBlockItem && startKnown && lengthKnown {
					length = max(length, itemStart+itemLength)
				}
				if depth == 0 {
					closed = true
				}
			}
			continue
		}
		if depth == 0 {
			continue
		}
		switch stack[depth-1] {
		case projectBlockRoot:
			if values, ok := lineHasKey(line, projectTempoKey); ok && baseTempo == 0 {
				if tempo, ok := firstNumber(values); ok && tempo >= 1 && tempo <= maxProjectFactsTempo {
					baseTempo = tempo
				}
			} else if values, ok := lineHasKey(line, projectMarkerKey); ok && collector != nil {
				collector.marker(values)
			}
		case projectBlockTempo:
			if values, ok := lineHasKey(line, projectTempoPointKey); ok {
				fields := bytes.Fields(values)
				if len(fields) >= 2 {
					if tempo, ok := projectNumber(fields[1]); ok && tempo >= 1 && tempo <= maxProjectFactsTempo {
						if !pointSeen {
							pointMin, pointMax, pointSeen = tempo, tempo, true
						} else {
							pointMin, pointMax = min(pointMin, tempo), max(pointMax, tempo)
						}
					}
				}
			}
		case projectBlockItem:
			if values, ok := lineHasKey(line, projectItemPositionKey); ok {
				itemStart, startKnown = firstNumber(values)
			} else if values, ok := lineHasKey(line, projectItemLengthKey); ok {
				itemLength, lengthKnown = firstNumber(values)
			}
		case projectBlockTrack:
			if values, ok := lineHasKey(line, projectNameKey); ok && collector != nil && !trackNamed {
				trackNamed = true
				collector.track(values)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return ProjectFactsTooLarge, ProjectFacts{}, nil
		}
		return ProjectFactsFailed, ProjectFacts{}, nil
	}
	if counted.n > maxBytes {
		return ProjectFactsTooLarge, ProjectFacts{}, nil
	}
	if !started || !closed {
		return ProjectFactsUnreadable, ProjectFacts{}, nil
	}

	facts := ProjectFacts{TempoBPM: math.Round(baseTempo*10) / 10}
	if baseTempo != 0 && pointSeen {
		low, high := min(pointMin, facts.TempoBPM), max(pointMax, facts.TempoBPM)
		facts.TempoVaries = math.Round(high*10) != math.Round(low*10)
	}
	if rounded := math.Round(length*1000) / 1000; rounded <= maxProjectFactsLength {
		facts.LengthSeconds = rounded
	}
	if tracks <= maxProjectFactsTracks {
		facts.TrackCount = tracks
	}
	if collector == nil {
		return ProjectFactsRead, facts, nil
	}
	return ProjectFactsRead, facts, collector.result()
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// lineHasKey reports whether line starts with key followed by a space or a
// tab, and returns what follows the key.
func lineHasKey(line, key []byte) ([]byte, bool) {
	if len(line) <= len(key) || !bytes.HasPrefix(line, key) || (line[len(key)] != ' ' && line[len(key)] != '\t') {
		return nil, false
	}
	return line[len(key):], true
}

// firstNumber parses the first value of bytes.Fields as a number.
func firstNumber(values []byte) (float64, bool) {
	fields := bytes.Fields(values)
	if len(fields) == 0 {
		return 0, false
	}
	return projectNumber(fields[0])
}

// projectNumber accepts a value of 1 to 32 bytes that strconv.ParseFloat
// accepts and that is finite and not negative.
func projectNumber(value []byte) (float64, bool) {
	if len(value) < 1 || len(value) > maxProjectFactsNumberBytes {
		return 0, false
	}
	number, err := strconv.ParseFloat(string(value), 64)
	if err != nil || math.IsInf(number, 0) || math.IsNaN(number) || number < 0 {
		return 0, false
	}
	return number, true
}

// nameCollector keeps the capped name lists in project order.
type nameCollector struct {
	tracks        []string
	tracksCost    int
	tracksLeftOut int
	markers       []collectedMarker
	markersCost   int
	markerLeftOut int
	// cutRegion is the number of the latest region start that a cap left
	// out, so that its end line is not counted a second time.
	cutRegion    uint32
	cutRegionSet bool
}

type collectedMarker struct {
	ProjectMarker
	number uint32
	open   bool
}

func (c *nameCollector) track(values []byte) {
	var first [1][]byte
	count, ok := nameLineValues(values, first[:])
	if !ok || count < 1 {
		return
	}
	name, ok := songName(first[0])
	if !ok {
		return
	}
	cost := songNameCost(name)
	if len(c.tracks)+1 > maxProjectFactsNames || c.tracksCost+cost > maxProjectFactsNameCost {
		c.tracksLeftOut = min(c.tracksLeftOut+1, maxProjectFactsLeftOut)
		return
	}
	c.tracks = append(c.tracks, name)
	c.tracksCost += cost
}

func (c *nameCollector) marker(values []byte) {
	var parts [4][]byte
	count, ok := nameLineValues(values, parts[:])
	if !ok || count < 4 {
		return
	}
	if len(parts[0]) > maxProjectFactsMarkerID {
		return
	}
	number, err := strconv.ParseUint(string(parts[0]), 10, 32)
	if err != nil {
		return
	}
	at, ok := projectNumber(parts[1])
	if !ok || at > maxProjectFactsLength {
		return
	}
	at = math.Round(at*1000) / 1000
	flags, err := strconv.ParseUint(string(parts[3]), 10, 64)
	if err != nil {
		return
	}
	id := uint32(number)
	region := flags&1 != 0
	if region {
		if c.cutRegionSet && c.cutRegion == id {
			c.cutRegionSet = false
			return
		}
		for i := len(c.markers) - 1; i >= 0; i-- {
			if c.markers[i].open && c.markers[i].number == id {
				if at > c.markers[i].At {
					c.markers[i].End = at
				}
				c.markers[i].open = false
				return
			}
		}
	}
	name, named := songName(parts[2])
	cost := 0
	if named {
		cost = songNameCost(name)
	}
	if len(c.markers)+1 > maxProjectFactsNames || c.markersCost+cost > maxProjectFactsNameCost {
		c.markerLeftOut = min(c.markerLeftOut+1, maxProjectFactsLeftOut)
		if region {
			c.cutRegion, c.cutRegionSet = id, true
		}
		return
	}
	entry := collectedMarker{ProjectMarker: ProjectMarker{At: at}, number: id, open: region}
	if named {
		entry.Name = name
	}
	c.markers = append(c.markers, entry)
	c.markersCost += cost
}

func (c *nameCollector) result() *ProjectNames {
	result := &ProjectNames{
		Tracks:         make([]string, 0, len(c.tracks)),
		TracksLeftOut:  c.tracksLeftOut,
		Markers:        make([]ProjectMarker, 0, len(c.markers)),
		MarkersLeftOut: c.markerLeftOut,
	}
	result.Tracks = append(result.Tracks, c.tracks...)
	for _, marker := range c.markers {
		result.Markers = append(result.Markers, marker.ProjectMarker)
	}
	return result
}

// nameLineValues splits the values of a name or marker line on spaces and
// tabs. A value that opens with a double quote, a single quote or a backtick
// runs to the next copy of that character, with no escapes, and that closing
// character must be followed by a space, a tab or the end of the line. It
// fills out with the first values, returns how many values the line has, and
// reports false when a quote never closes or is followed by anything else, in
// which case the whole line is skipped.
func nameLineValues(line []byte, out [][]byte) (int, bool) {
	count := 0
	i := 0
	for {
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i == len(line) {
			return count, true
		}
		var value []byte
		switch quote := line[i]; quote {
		case '"', '\'', '`':
			end := bytes.IndexByte(line[i+1:], quote)
			if end < 0 {
				return 0, false
			}
			value = line[i+1 : i+1+end]
			i += end + 2
			if i < len(line) && line[i] != ' ' && line[i] != '\t' {
				return 0, false
			}
		default:
			start := i
			for i < len(line) && line[i] != ' ' && line[i] != '\t' {
				i++
			}
			value = line[start:i]
		}
		if count < len(out) {
			out[count] = value
		}
		count++
	}
}

// songName cleans one name: invalid UTF-8, control characters and the line
// and bidirectional-override separators are dropped, leading white space is
// skipped, at most 80 characters are kept and trailing white space is
// trimmed. It reports false when nothing is left or the name is shaped like a
// path.
func songName(raw []byte) (string, bool) {
	var builder strings.Builder
	kept := 0
	for i := 0; i < len(raw) && kept < maxProjectFactsNameRunes; {
		r, size := utf8.DecodeRune(raw[i:])
		i += size
		if r == utf8.RuneError && size == 1 {
			continue
		}
		if unicode.IsControl(r) || r == ' ' || r == ' ' || (r >= '‪' && r <= '‮') || (r >= '⁦' && r <= '⁩') {
			continue
		}
		if kept == 0 && unicode.IsSpace(r) {
			continue
		}
		builder.WriteRune(r)
		kept++
	}
	name := strings.TrimRightFunc(builder.String(), unicode.IsSpace)
	if name == "" || pathShapedName(name) {
		return "", false
	}
	return name, true
}

// songNameCost is a name's byte length with the characters a JSON encoder
// escapes counted at their escaped length.
func songNameCost(name string) int {
	cost := 0
	for i := 0; i < len(name); i++ {
		switch name[i] {
		case '"', '\\':
			cost += 2
		case '<', '>', '&':
			cost += 6
		default:
			cost++
		}
	}
	return cost
}

// pathShapedName reports a name that reads as a file path. "Vox L/R" and
// "AC/DC" are names.
func pathShapedName(name string) bool {
	switch name {
	case ".", "..", "~":
		return true
	}
	for _, prefix := range []string{"/", "~/", "./", "../"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	for _, part := range []string{"\\", " /", " ~/", "(/"} {
		if strings.Contains(name, part) {
			return true
		}
	}
	if len(name) >= 3 && name[1] == ':' && (name[2] == '/' || name[2] == '\\') {
		c := name[0]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			return true
		}
	}
	return false
}

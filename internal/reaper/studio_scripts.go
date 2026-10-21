package reaper

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/text/unicode/norm"
)

// Script entries carry three facts and an optional description, each only when
// inspected metadata backs it (contract §4.2, §4.3):
//
//   - language, from the file extension;
//   - registration, from reaper-kb.ini's SCR rows matched by script path
//     ("Yes"/"No"), omitted when that file was not inspected;
//   - package, when the script's top folder is a ReaPack repository named in
//     reapack.ini, omitted otherwise;
//   - description, from a tagged header line within the first 8 KiB.
//
// Nothing here runs a script. A registration row that points outside REAPER's
// Scripts folder and the Ori library is counted as external_registration on
// the Scripts source's first page and never followed. That page is not told
// the owner's folders (§4.5), so a row that points into one is still counted
// there; the folder's own page reports the script as registered.

const (
	maxStudioHeaderBytes    = 8192
	maxStudioHeaderLines    = 200
	maxReaPackConfigBytes   = 1 << 20
	studioDescriptionSource = "Script header"
	studioRegisteredYes     = "Yes"
	studioRegisteredNo      = "No"
)

var studioLanguages = map[string]string{".lua": "Lua", ".eel": "EEL", ".py": "Python"}

type studioScriptFacts struct {
	spec       studioSourceSpec
	sourcePath string

	registrationsInspected bool
	relativeRows           map[string]struct{}
	absoluteRows           map[string]struct{}
	external               int

	remotes map[string]string
}

// newStudioScriptFacts prepares the metadata a scripts source is matched
// against. It is nil for sources that carry no facts.
func newStudioScriptFacts(roots *studioRoots, spec studioSourceSpec, dir *os.Root) *studioScriptFacts {
	if roots == nil || dir == nil || spec.Category != StudioCategoryScripts {
		return nil
	}
	facts := &studioScriptFacts{spec: spec, sourcePath: filepath.Clean(dir.Name())}
	if roots.resource != nil && roots.resourceState == StudioStateReady {
		facts.loadRegistrations(roots)
		// ReaPack installs under REAPER's Scripts folder only, so the package
		// fact is backed there and nowhere else.
		if spec.reaperScripts() {
			facts.loadRemotes(roots.resource)
		}
	}
	return facts
}

func (f *studioScriptFacts) Decorate(entry *StudioEntry, dir *os.Root, name, relative string) {
	if f == nil || entry == nil {
		return
	}
	if language, ok := studioLanguages[strings.ToLower(filepath.Ext(name))]; ok {
		entry.Facts = append(entry.Facts, StudioFact{ID: StudioFactLanguage, Value: language})
	}
	if f.registrationsInspected {
		value := studioRegisteredNo
		if f.registered(relative) {
			value = studioRegisteredYes
		}
		entry.Facts = append(entry.Facts, StudioFact{ID: StudioFactRegistration, Value: value})
	}
	if pkg := f.packageOf(relative); pkg != "" {
		entry.Facts = append(entry.Facts, StudioFact{ID: StudioFactPackage, Value: pkg})
	}
	if text := readStudioHeaderDescription(dir, name); text != "" {
		entry.Description = &StudioDescription{Text: text, Source: studioDescriptionSource}
	}
}

// loadRegistrations reads reaper-kb.ini as metadata only: the path field of
// each SCR row, relative to REAPER's Scripts folder or absolute.
func (f *studioScriptFacts) loadRegistrations(roots *studioRoots) {
	data, ok := readStudioResourceFile(roots.resource, "reaper-kb.ini", maxKeyboardConfigBytes)
	if !ok {
		return
	}
	f.registrationsInspected = true
	f.relativeRows = map[string]struct{}{}
	f.absoluteRows = map[string]struct{}{}
	scriptsPath := filepath.Join(roots.resourcePath, "Scripts")
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), maxKeyboardLineBytes)
	lines := 0
	for scanner.Scan() {
		lines++
		if lines > maxKeyboardConfigLines {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "SCR ") {
			continue
		}
		fields, err := parseKeyboardFields(line)
		if err != nil || len(fields) < 6 || fields[0] != "SCR" {
			continue
		}
		path := strings.TrimSpace(fields[5])
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			f.relativeRows[studioPathKey(path)] = struct{}{}
			continue
		}
		path = filepath.Clean(path)
		f.absoluteRows[studioPathKey(path)] = struct{}{}
		if !pathInsideLexically(path, scriptsPath) && (roots.libraryPath == "" || !pathInsideLexically(path, roots.libraryPath)) {
			f.external++
		}
	}
}

func (f *studioScriptFacts) registered(relative string) bool {
	if !f.registrationsInspected {
		return false
	}
	// Relative rows are relative to REAPER's Scripts folder; every other
	// source, the library and an owner folder, is matched by absolute path.
	if f.spec.reaperScripts() {
		if _, ok := f.relativeRows[studioPathKey(relative)]; ok {
			return true
		}
	}
	_, ok := f.absoluteRows[studioPathKey(filepath.Join(f.sourcePath, filepath.FromSlash(relative)))]
	return ok
}

// studioPathKey compares paths the way the file system does here: NFC,
// forward slashes, without regard to case.
func studioPathKey(path string) string {
	return strings.ToLower(norm.NFC.String(filepath.ToSlash(path)))
}

// loadRemotes reads the repository names ReaPack keeps in reapack.ini
// ([remotes], remoteN=Name|URL|…). ReaPack installs a repository's scripts
// under Scripts/<repository name>/, which is the only package evidence v1 uses.
func (f *studioScriptFacts) loadRemotes(resource *os.Root) {
	data, ok := readStudioResourceFile(resource, "reapack.ini", maxReaPackConfigBytes)
	if !ok {
		return
	}
	f.remotes = map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 64<<10)
	inRemotes := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "["):
			inRemotes = strings.EqualFold(line, "[remotes]")
		case inRemotes && strings.HasPrefix(strings.ToLower(line), "remote"):
			_, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			name, _, _ := strings.Cut(value, "|")
			name = strings.TrimSpace(norm.NFC.String(name))
			if profileLine(name, maxStudioFactValueChars) && !strings.ContainsAny(name, `/\`) {
				f.remotes[strings.ToLower(name)] = name
			}
		}
	}
}

func (f *studioScriptFacts) packageOf(relative string) string {
	if len(f.remotes) == 0 {
		return ""
	}
	top, _, ok := strings.Cut(relative, "/")
	if !ok {
		return ""
	}
	return f.remotes[strings.ToLower(top)]
}

// readStudioResourceFile reads one bounded regular file directly inside the
// opened resource folder. A link, another type, an oversized or unreadable
// file reads as not inspected.
func readStudioResourceFile(resource *os.Root, name string, limit int64) ([]byte, bool) {
	if resource == nil {
		return nil, false
	}
	before, err := resource.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || before.Size() > limit {
		return nil, false
	}
	file, err := resource.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, false
	}
	defer func() { _ = file.Close() }()
	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, false
	}
	return data, true
}

// readStudioHeaderDescription reads the first bytes of one script through its
// opened folder and returns a tagged description: a ReaPack "@description"
// line, the "description:" of an Ori frontmatter block, or an EEL "desc:"
// line. An untagged comment is not a description.
func readStudioHeaderDescription(dir *os.Root, name string) string {
	if dir == nil {
		return ""
	}
	before, err := dir.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || before.Size() == 0 {
		return ""
	}
	file, err := dir.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()
	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(file, maxStudioHeaderBytes))
	if err != nil && !errors.Is(err, io.EOF) {
		return ""
	}
	return studioHeaderDescription(data)
}

func studioHeaderDescription(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	if bytes.HasPrefix(data, []byte("--[[ori")) {
		if metadata := ParseFrontmatter(data); metadata.Valid {
			return studioDescriptionText(metadata.Description)
		}
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), maxStudioHeaderBytes+1)
	lines := 0
	for scanner.Scan() {
		lines++
		if lines > maxStudioHeaderLines {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if index := strings.Index(lower, "@description"); index >= 0 && studioCommentLine(line) {
			if text := studioDescriptionText(line[index+len("@description"):]); text != "" {
				return text
			}
		}
		if strings.HasPrefix(lower, "desc:") {
			if text := studioDescriptionText(line[len("desc:"):]); text != "" {
				return text
			}
		}
	}
	return ""
}

func studioCommentLine(line string) bool {
	for _, marker := range []string{"--", "//", "#", "/*", "*", "[[", "@description"} {
		if strings.HasPrefix(line, marker) {
			return true
		}
	}
	return false
}

func studioDescriptionText(value string) string {
	text := strings.TrimSpace(norm.NFC.String(strings.TrimSpace(value)))
	text = strings.TrimRight(text, "]")
	text = strings.TrimSpace(text)
	if !profileLine(text, maxStudioDescriptionChars) {
		return ""
	}
	return text
}

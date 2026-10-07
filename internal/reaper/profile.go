package reaper

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// The profile read is the one thing this plugin tells a host about the
// computer rather than about a project: whether REAPER is installed, which
// version, and (only when the host asks, after the user agreed) the names of
// the user's project and track templates. It opens no template, follows no
// symbolic link, and returns names only: never a path.

const (
	profileAppName = "REAPER"

	ProfileTemplateProject = "project"
	ProfileTemplateTrack   = "track"

	// The bounds below are the ones the manifest declares for profile.read's
	// output; TestProfileBoundsAreTheOnesTheManifestDeclares holds the two
	// together.
	//
	// maxProfileTemplates is the most template names one read returns; a
	// longer list is cut and reported as truncated.
	maxProfileTemplates     = 64
	maxProfileTemplateName  = 120
	maxProfileTemplateFile  = 255
	maxProfileVersionLength = 40
	maxProfileModifiedAt    = 40
	// maxProfileOutputBytes is the declared size limit of one answer. A host
	// refuses a larger one whole, so the list is cut to fit instead.
	maxProfileOutputBytes = 32768
	// profileOutputReserve is kept free of template entries for the rest of
	// the answer and for how a host may re-encode it.
	profileOutputReserve = 1024

	// maxProfileDirEntries bounds how much of one template folder is looked at.
	maxProfileDirEntries = 4096
	// maxBundleInfoBytes bounds the application metadata file a version is
	// read from.
	maxBundleInfoBytes = 256 << 10
)

// profileTemplateFolders are the two folders of a REAPER resource folder that
// hold the user's templates, the file extension each one uses, and the kind a
// host knows them by. Extensions are matched without regard to case.
var profileTemplateFolders = []struct {
	Folder    string
	Extension string
	Kind      string
}{
	{"ProjectTemplates", ".rpp", ProfileTemplateProject},
	{"TrackTemplates", ".rtracktemplate", ProfileTemplateTrack},
}

type ProfileInput struct {
	// IncludeTemplates asks for template names. Without it the read reports
	// installation and version only and looks at no template folder's contents.
	IncludeTemplates bool `json:"include_templates"`
}

// ProfileTemplate is one template by name. File is the bare file name inside
// its template folder, so a host can tell two templates apart; it is not a
// path and cannot be used as one.
type ProfileTemplate struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	File       string `json:"file"`
	ModifiedAt string `json:"modified_at,omitempty"`
}

type ProfileResult struct {
	App       string `json:"app"`
	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
	// TemplatesAvailable says a template folder exists to be listed. It is
	// reported with or without IncludeTemplates.
	TemplatesAvailable bool              `json:"templates_available"`
	Templates          []ProfileTemplate `json:"templates,omitempty"`
	// Truncated says the list is not everything: the folders hold more
	// templates than were returned, some could not be named within the bounds,
	// some sit in subfolders (which are not listed), or a folder could not be
	// read.
	Truncated bool `json:"truncated"`
}

// ProfileProbe reads the platform's facts for a profile.
type ProfileProbe interface {
	ReadProfile(ctx context.Context, includeTemplates bool) ProfileResult
}

// Profile answers the profile.read operation. It is machine-level: no
// workspace, project or grant takes part, and nothing is opened or changed.
// Whatever the platform probe reports is bounded again here, in count, in
// every field and in total size, so the answer always fits what the manifest
// declares and a host never has to refuse it whole.
func (s *Service) Profile(ctx context.Context, input ProfileInput) ProfileResult {
	result := ProfileResult{App: profileAppName}
	if s == nil || s.profile == nil {
		return result
	}
	observed := s.profile.ReadProfile(ctx, input.IncludeTemplates)
	if !observed.Installed {
		return result
	}
	result.Installed = true
	result.Version = profileVersion(observed.Version)
	result.TemplatesAvailable = observed.TemplatesAvailable
	if !input.IncludeTemplates || !observed.TemplatesAvailable {
		return result
	}
	result.Truncated = observed.Truncated
	budget := maxProfileOutputBytes - profileOutputReserve
	for _, template := range observed.Templates {
		if !validProfileTemplate(template) {
			result.Truncated = true
			continue
		}
		if !validProfileModifiedAt(template.ModifiedAt) {
			template.ModifiedAt = ""
		}
		// The size is counted the way a host counts it: the JSON encoding,
		// in which a few characters take six bytes each.
		encoded, err := json.Marshal(template)
		if err != nil {
			result.Truncated = true
			continue
		}
		if len(result.Templates) >= maxProfileTemplates || len(encoded)+1 > budget {
			result.Truncated = true
			break
		}
		budget -= len(encoded) + 1
		result.Templates = append(result.Templates, template)
	}
	return result
}

func validProfileTemplate(template ProfileTemplate) bool {
	return (template.Kind == ProfileTemplateProject || template.Kind == ProfileTemplateTrack) &&
		profileLine(template.Name, maxProfileTemplateName) && profileFileName(template.File)
}

// validProfileModifiedAt accepts no time, or one written the only way this
// plugin writes it.
func validProfileModifiedAt(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > maxProfileModifiedAt {
		return false
	}
	_, err := time.Parse(time.RFC3339, value)
	return err == nil
}

// profileLine accepts one trimmed line of plain, visible text within limit
// characters. Besides control characters it refuses the characters that
// could make a name read differently from what it is: line and paragraph
// separators, and the marks that reorder text. A name with nothing visible in
// it is refused too.
func profileLine(value string, limit int) bool {
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > limit || strings.TrimSpace(value) != value {
		return false
	}
	visible := false
	for _, r := range value {
		switch {
		case unicode.IsControl(r), unicode.Is(unicode.Zl, r), unicode.Is(unicode.Zp, r), unicode.Is(unicode.Bidi_Control, r):
			return false
		case unicode.IsGraphic(r) && !unicode.IsSpace(r) && !unicode.Is(unicode.Cf, r):
			visible = true
		}
	}
	return visible
}

// profileFileName accepts a bare file name: one path element, nothing that a
// host could read as a directory, a parent reference or a volume.
func profileFileName(file string) bool {
	return profileLine(file, maxProfileTemplateFile) && file != "." && file != ".." &&
		!strings.ContainsAny(file, `/\:`)
}

var (
	profileVersionNumber = regexp.MustCompile(`^[0-9]{1,4}\.[0-9]{1,4}`)
	profileVersionOther  = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+ -]{0,39}$`)
)

// profileVersion returns the version the way REAPER names its releases
// ("7.28"), from the longer string an application bundle carries
// ("7.28.0_1a2b3c4d"). A string that does not start with a release number is
// returned as it is when it is one short word-like value, and dropped
// otherwise.
func profileVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	if number := profileVersionNumber.FindString(raw); number != "" {
		return number
	}
	if len(raw) <= maxProfileVersionLength && profileVersionOther.MatchString(raw) {
		return raw
	}
	return ""
}

// parseBundleVersion reads the short version string from an application
// bundle's XML property list: the value of that key in the list's own
// dictionary, not one nested further down. Any other format (a binary list, a
// damaged file) reads as no version; it is never an error.
func parseBundleVersion(data []byte) string {
	if len(data) == 0 || len(data) > maxBundleInfoBytes || bytes.HasPrefix(data, []byte("bplist")) {
		return ""
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = false
	// The list is read as tokens and nothing in it is resolved: no entity, no
	// external reference.
	decoder.Entity = map[string]string{}
	// plist > dict > key|string: the entries of the top dictionary sit at
	// depth 3.
	const entryDepth = 3
	var short, build, key string
	var text strings.Builder
	depth, reading := 0, false
	for {
		token, err := decoder.Token()
		if err != nil {
			// The end of the list, or a damaged one: what was read before it
			// stands.
			return profileVersion(firstNonEmpty(short, build))
		}
		switch value := token.(type) {
		case xml.StartElement:
			depth++
			reading = depth == entryDepth && (value.Name.Local == "key" || value.Name.Local == "string")
			text.Reset()
		case xml.CharData:
			if reading {
				if text.Len()+len(value) > maxBundleInfoBytes {
					return ""
				}
				text.Write(value)
			}
		case xml.EndElement:
			if depth == entryDepth {
				content := strings.TrimSpace(text.String())
				switch value.Name.Local {
				case "key":
					key = content
				case "string":
					// The first value of each key is the one that counts.
					if key == "CFBundleShortVersionString" && short == "" {
						short = content
					}
					if key == "CFBundleVersion" && build == "" {
						build = content
					}
					key = ""
				default:
					// A value of another type belongs to the key before it.
					key = ""
				}
			}
			depth--
			reading = false
			text.Reset()
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// openRealDir opens the directory called element inside parent, and only a
// real one. exists says a real directory of that name is there; ok says it
// was opened. A symbolic link is never followed: one found by the look is
// refused, and one swapped in between the look and the open is refused too,
// because what was opened must be the very directory that was looked at.
// Everything below is then reached through the opened directory itself, not
// by walking its path again.
func openRealDir(parent *os.Root, element string) (dir *os.Root, exists, ok bool) {
	// One plain path element, nothing that names another place.
	if parent == nil || element == "" || element == "." || element == ".." || strings.ContainsAny(element, `/\`) {
		return nil, false, false
	}
	before, err := parent.Lstat(element)
	if err != nil || before.Mode()&os.ModeSymlink != 0 || !before.IsDir() {
		return nil, false, false
	}
	// From here on a real directory of that name was seen. Whatever keeps it
	// from being used (no permission, or something else now in its place) is
	// "there, not readable", never "absent".
	child, err := parent.OpenRoot(element)
	if err != nil {
		return nil, true, false
	}
	after, err := child.Stat(".")
	if err != nil || !after.IsDir() || !os.SameFile(before, after) {
		_ = child.Close()
		return nil, true, false
	}
	return child, true, true
}

// regularFileIn says a regular file of that name sits directly in dir. A
// symbolic link is not one, whatever it points at. The file is not opened.
func regularFileIn(dir *os.Root, name string) bool {
	if dir == nil {
		return false
	}
	info, err := dir.Lstat(name)
	return err == nil && info.Mode().IsRegular()
}

// profileTemplatesAvailable says whether a REAPER resource folder has a
// template folder to list. It looks at no folder's contents.
func profileTemplatesAvailable(resource *os.Root) bool {
	if resource == nil {
		return false
	}
	for _, folder := range profileTemplateFolders {
		if info, err := resource.Lstat(folder.Folder); err == nil && info.Mode()&os.ModeSymlink == 0 && info.IsDir() {
			return true
		}
	}
	return false
}

// listProfileTemplates names the template files directly inside the two
// template folders of one REAPER resource folder: project templates first,
// then track templates, each in name order. Only directory entries are read;
// no template is opened. Hidden files and files of another type are passed
// over. The list is marked truncated when it is not everything a user would
// call their templates: a folder or a list longer than the limits, a file
// whose name cannot be reported within the bounds, a template that is a
// symbolic link, a subfolder (REAPER shows what is in it, this read does not
// look), or a folder or entry that could not be read.
func listProfileTemplates(resource *os.Root) (templates []ProfileTemplate, available, truncated bool) {
	if resource == nil {
		return nil, false, false
	}
	for _, folder := range profileTemplateFolders {
		dir, exists, ok := openRealDir(resource, folder.Folder)
		if !exists {
			continue
		}
		available = true
		if !ok {
			// It is there and cannot be opened: not an empty folder.
			truncated = true
			continue
		}
		found, more := readProfileTemplateFolder(dir, folder.Extension, folder.Kind)
		_ = dir.Close()
		truncated = truncated || more
		sort.SliceStable(found, func(i, j int) bool {
			left, right := strings.ToLower(found[i].Name), strings.ToLower(found[j].Name)
			if left != right {
				return left < right
			}
			return found[i].File < found[j].File
		})
		templates = append(templates, found...)
	}
	if len(templates) > maxProfileTemplates {
		templates, truncated = templates[:maxProfileTemplates], true
	}
	return templates, available, truncated
}

func readProfileTemplateFolder(dir *os.Root, extension, kind string) (templates []ProfileTemplate, truncated bool) {
	handle, err := dir.Open(".")
	if err != nil {
		return nil, true
	}
	defer func() { _ = handle.Close() }()
	entries, err := handle.ReadDir(maxProfileDirEntries)
	if err != nil && !errors.Is(err, io.EOF) {
		// What was read before the failure is kept; the list is not complete.
		truncated = true
	}
	if len(entries) == maxProfileDirEntries {
		if rest, _ := handle.ReadDir(1); len(rest) > 0 {
			truncated = true
		}
	}
	for _, entry := range entries {
		file := entry.Name()
		if strings.HasPrefix(file, ".") {
			continue
		}
		if entry.IsDir() {
			truncated = true
			continue
		}
		if !strings.EqualFold(filepath.Ext(file), extension) {
			continue
		}
		// The entry's own metadata, read through the opened folder: a
		// symbolic link is reported as a link and never followed.
		info, err := dir.Lstat(file)
		if err != nil || !info.Mode().IsRegular() {
			truncated = true
			continue
		}
		template := ProfileTemplate{
			Name: strings.TrimSpace(file[:len(file)-len(filepath.Ext(file))]), Kind: kind, File: file,
		}
		if !validProfileTemplate(template) {
			truncated = true
			continue
		}
		if modified := info.ModTime().UTC(); modified.Year() >= 1970 && modified.Year() <= 9998 {
			template.ModifiedAt = modified.Format(time.RFC3339)
		}
		templates = append(templates, template)
	}
	return templates, truncated
}

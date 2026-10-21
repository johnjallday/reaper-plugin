package reaper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// The REAPER Studio catalog (Ori contract reaper-studio v1, §4) is two
// machine-level read-only operations. studio.sources.read says which resource
// folders this computer has and whether REAPER's plugin list can be read now;
// studio.inventory.read lists one of those sources page by page. Neither one
// creates a folder, opens anything for writing, follows a link, runs a
// cataloged script, applies a template or loads a plugin. Ori owns the catalog,
// the annotations and every workspace; this plugin only observes.

const (
	StudioSchemaVersion = 1

	StudioAccessFile = "file"
	StudioAccessLive = "live"

	StudioLayoutPerUser  = "per_user"
	StudioLayoutPortable = "portable"

	StudioStateReady               = "ready"
	StudioStateOK                  = "ok"
	StudioStateAbsent              = "absent"
	StudioStateDenied              = "denied"
	StudioStateUnsupported         = "unsupported"
	StudioStateAppClosed           = "app_closed"
	StudioStateReaderMissing       = "reader_missing"
	StudioStateReaderOutdated      = "reader_outdated"
	StudioStateWebRemoteOff        = "web_remote_off"
	StudioStateInstallationChanged = "installation_changed"
	StudioStateCursorReset         = "cursor_reset"

	StudioCategoryTemplates    = "templates"
	StudioCategoryScripts      = "scripts"
	StudioCategoryAudioPlugins = "audio_plugins"

	StudioKindProjectTemplate = "project_template"
	StudioKindTrackTemplate   = "track_template"
	StudioKindScript          = "script"
	StudioKindEffect          = "effect"
	StudioKindInstrument      = "instrument"

	StudioFactLanguage     = "language"
	StudioFactRegistration = "registration"
	StudioFactPackage      = "package"
	StudioFactFormat       = "format"
	StudioFactVendor       = "vendor"

	// The bounds below are the ones the manifest declares for the two
	// operations; TestStudioBoundsAreTheOnesTheManifestDeclares holds them
	// together.
	maxStudioSourcesOutputBytes   = 32768
	maxStudioInventoryOutputBytes = 262144
	// studioInventoryOutputReserve is kept free of entries for the rest of the
	// answer and for how a host may re-encode it.
	studioInventoryOutputReserve = 4096
	maxStudioPageEntries         = 256
	maxStudioNativeIDBytes       = 512
	maxStudioNameChars           = 160
	maxStudioFactValueChars      = 160
	maxStudioDescriptionChars    = 500
	maxStudioDetailChars         = 240
	maxStudioCursorBytes         = 512
	minStudioInstallationIDChars = 8
	maxStudioInstallationIDChars = 128
	maxStudioSourceIDChars       = 40
	maxStudioSourceLabelChars    = 60
	maxStudioLocationChars       = 512
	studioInstallationIDPrefix   = "inst_"

	// Owner-added folders (contract §4.5): Ori sends at most eight per
	// computer, each named folder_ + 16 hex digits of
	// SHA-256(category ‖ 0x00 ‖ canonical path).
	studioFolderSourcePrefix = "folder_"
	studioFolderIDHexChars   = 16
	maxStudioOwnerFolders    = 8
	maxStudioFolderPathChars = 1024
)

// Coverage reasons a page may report (contract §4.2).
const (
	studioSkipSymlink              = "symlink"
	studioSkipOutsideSource        = "outside_source"
	studioSkipUnreadable           = "unreadable"
	studioSkipUnsupportedName      = "unsupported_name"
	studioSkipTooLarge             = "too_large"
	studioSkipHidden               = "hidden"
	studioSkipExternalRegistration = "external_registration"
	studioSkipDepthLimit           = "depth_limit"
)

// StudioFolderInput is one folder the owner added to a category, as Ori sends
// it in studio.sources.read (contract §4.5): its folder_ id, the category and
// the canonical absolute path Ori checked when the folder was added.
type StudioFolderInput struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Path     string `json:"path"`
}

type StudioSourcesInput struct {
	SchemaVersion int                 `json:"schema_version"`
	Folders       []StudioFolderInput `json:"folders,omitempty"`
}

// StudioFolderRef names the owner folder an inventory page reads. It is
// present exactly when the page's source_id is a folder_ id.
type StudioFolderRef struct {
	Category string `json:"category"`
	Path     string `json:"path"`
}

type StudioInstallation struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Layout       string `json:"layout"`
	AppInstalled bool   `json:"app_installed"`
	AppVersion   string `json:"app_version,omitempty"`
}

type StudioSource struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Label    string `json:"label"`
	Location string `json:"location"`
	Access   string `json:"access"`
	State    string `json:"state"`
	Detail   string `json:"detail,omitempty"`
}

type StudioSourcesResult struct {
	SchemaVersion int                `json:"schema_version"`
	Installation  StudioInstallation `json:"installation"`
	Sources       []StudioSource     `json:"sources"`
}

type StudioInventoryInput struct {
	SchemaVersion  int              `json:"schema_version"`
	InstallationID string           `json:"installation_id"`
	SourceID       string           `json:"source_id"`
	Cursor         string           `json:"cursor"`
	Limit          int              `json:"limit"`
	Folder         *StudioFolderRef `json:"folder,omitempty"`
}

type StudioFact struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

type StudioDescription struct {
	Text   string `json:"text"`
	Source string `json:"source"`
}

type StudioEntry struct {
	NativeID    string             `json:"native_id"`
	Kind        string             `json:"kind"`
	Name        string             `json:"name"`
	ModifiedAt  string             `json:"modified_at,omitempty"`
	Facts       []StudioFact       `json:"facts"`
	Description *StudioDescription `json:"description,omitempty"`
}

type StudioSkip struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

type StudioCoverage struct {
	Visited int          `json:"visited"`
	Skipped []StudioSkip `json:"skipped"`
}

type StudioInventoryResult struct {
	SchemaVersion  int            `json:"schema_version"`
	InstallationID string         `json:"installation_id"`
	SourceID       string         `json:"source_id"`
	State          string         `json:"state"`
	Entries        []StudioEntry  `json:"entries"`
	NextCursor     string         `json:"next_cursor"`
	Coverage       StudioCoverage `json:"coverage"`
}

// studioSourceSpec is one source: one of the five fixed v1 sources (contract
// §4.1) or a folder the owner added to a category (§4.5).
type studioSourceSpec struct {
	ID       string
	Category string
	Label    string
	Access   string
	// Folder names the sub-folder of the REAPER resource folder a file source
	// lives in. The Ori script library lives outside it (Library), and an
	// owner folder is wherever its canonical Path says (Owner).
	Folder  string
	Library bool
	Owner   bool
	Path    string
	// Kinds maps a lower-case file extension to the kind a file of it lists as.
	Kinds map[string]string
}

var (
	studioTemplateKinds = map[string]string{".rpp": StudioKindProjectTemplate, ".rtracktemplate": StudioKindTrackTemplate}
	studioScriptKinds   = map[string]string{".lua": StudioKindScript, ".eel": StudioKindScript, ".py": StudioKindScript}
	// studioOwnerFolderKinds are the categories that declare owner_folders
	// (§3.4) with the kinds a folder of each lists by extension (§4.5).
	studioOwnerFolderKinds = map[string]map[string]string{
		StudioCategoryTemplates: studioTemplateKinds,
		StudioCategoryScripts:   studioScriptKinds,
	}
)

var studioSourceSpecs = []studioSourceSpec{
	{ID: "project_templates", Category: StudioCategoryTemplates, Label: "Project templates", Access: StudioAccessFile,
		Folder: "ProjectTemplates", Kinds: map[string]string{".rpp": StudioKindProjectTemplate}},
	{ID: "track_templates", Category: StudioCategoryTemplates, Label: "Track templates", Access: StudioAccessFile,
		Folder: "TrackTemplates", Kinds: map[string]string{".rtracktemplate": StudioKindTrackTemplate}},
	{ID: "reaper_scripts", Category: StudioCategoryScripts, Label: "REAPER Scripts", Access: StudioAccessFile,
		Folder: "Scripts", Kinds: studioScriptKinds},
	{ID: "ori_scripts", Category: StudioCategoryScripts, Label: "Ori script library", Access: StudioAccessFile,
		Library: true, Kinds: studioScriptKinds},
	{ID: "audio_plugins", Category: StudioCategoryAudioPlugins, Label: "REAPER plugin list", Access: StudioAccessLive},
}

func lookupStudioSource(id string) (studioSourceSpec, bool) {
	for _, spec := range studioSourceSpecs {
		if spec.ID == id {
			return spec, true
		}
	}
	return studioSourceSpec{}, false
}

// kindOf is the kind a file of this name lists as in the source, if any.
func (spec studioSourceSpec) kindOf(name string) (string, bool) {
	kind, ok := spec.Kinds[strings.ToLower(filepath.Ext(name))]
	return kind, ok
}

// reaperScripts says the source is REAPER's own Scripts folder, the one
// reaper-kb.ini's relative registration rows are relative to.
func (spec studioSourceSpec) reaperScripts() bool {
	return spec.Access == StudioAccessFile && !spec.Library && !spec.Owner && spec.Folder == "Scripts"
}

// studioFolderID is an owner folder's source id (contract §4.5): the same
// category and canonical path always give the same id.
func studioFolderID(category, path string) string {
	sum := sha256.Sum256([]byte(category + "\x00" + path))
	return studioFolderSourcePrefix + hex.EncodeToString(sum[:studioFolderIDHexChars/2])
}

func isStudioFolderSource(id string) bool {
	return strings.HasPrefix(id, studioFolderSourcePrefix)
}

// studioFolderIDShape says an id has the shape of an owner folder id; whether
// it names the folder it came with is checked against the digest.
func studioFolderIDShape(id string) bool {
	if !isStudioFolderSource(id) || len(id) != len(studioFolderSourcePrefix)+studioFolderIDHexChars {
		return false
	}
	for i := len(studioFolderSourcePrefix); i < len(id); i++ {
		if c := id[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// studioValidFolderPath accepts the canonical absolute path of a folder: one
// plain line within the declared bound, clean, with nothing left to resolve,
// and not the root of the file system.
func studioValidFolderPath(path string) bool {
	return profileLine(path, maxStudioFolderPathChars) && filepath.IsAbs(path) && filepath.Clean(path) == path && path != string(filepath.Separator)
}

// studioOwnerFolderSpec is the source an owner folder reads as. problem is
// the state the folder must report instead when it is not one this plugin
// accepts: a category without owner folders, a path that is not canonical, or
// an id that is not the digest of its category and path (contract §4.5).
func studioOwnerFolderSpec(folder StudioFolderInput) (spec studioSourceSpec, problem, detail string) {
	spec = studioSourceSpec{ID: folder.ID, Category: folder.Category, Access: StudioAccessFile, Owner: true, Path: folder.Path, Label: studioFolderLabel(folder.Path)}
	kinds, ok := studioOwnerFolderKinds[folder.Category]
	switch {
	case !ok:
		return spec, StudioStateUnsupported, "Folders can be added to templates and scripts only."
	case !studioValidFolderPath(folder.Path):
		return spec, StudioStateUnsupported, "The folder's path is not a canonical absolute path."
	case !studioFolderIDShape(folder.ID) || studioFolderID(folder.Category, folder.Path) != folder.ID:
		return spec, StudioStateUnsupported, "The folder's id does not name this category and path."
	}
	spec.Kinds = kinds
	return spec, "", ""
}

// studioFolderLabel is an owner folder's label: its name, kept to one plain
// line within the bound a source label has.
func studioFolderLabel(path string) string {
	return studioClipLine(filepath.Base(path), maxStudioSourceLabelChars, "Folder")
}

// studioClipLine keeps a value to one plain line within limit characters, in
// NFC, cutting it when it is longer and falling back when nothing plain is
// left.
func studioClipLine(value string, limit int, fallback string) string {
	value = strings.TrimSpace(norm.NFC.String(value))
	if runes := []rune(value); len(runes) > limit {
		value = strings.TrimSpace(string(runes[:limit]))
	}
	if !profileLine(value, limit) {
		return fallback
	}
	return value
}

// studioRoots is what the platform probe opens for one read: the installation
// facts and the two folders sources live in, each held open as a real
// directory or marked with the state a source there must report. Close
// releases the handles.
type studioRoots struct {
	supported bool
	installed bool
	version   string
	layout    string
	// home is the user's home folder with links resolved, the way Ori
	// resolves the paths it sends; locations are shortened against it.
	home string

	resource        *os.Root
	resourceState   string
	resourcePath    string
	resourceDisplay string
	// identity is the resource folder's canonical path, and its filesystem
	// identity when the folder exists; together they make installation.id.
	identity    string
	dev, ino    uint64
	hasIdentity bool

	library        *os.Root
	libraryState   string
	libraryPath    string
	libraryDisplay string
}

func (r *studioRoots) Close() {
	if r == nil {
		return
	}
	closeRoot(r.resource)
	closeRoot(r.library)
	r.resource, r.library = nil, nil
}

func closeRoot(root *os.Root) {
	if root != nil {
		_ = root.Close()
	}
}

// isStudioFormattingRune marks the characters that could make a value read
// differently from what it is: line and paragraph separators and the marks
// that reorder text.
func isStudioFormattingRune(r rune) bool {
	return unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) || unicode.Is(unicode.Bidi_Control, r)
}

// InstallationID is stable for one REAPER resource folder on one computer:
// another resource folder, or the same path on another disk, gets a different
// ID (contract §4.1). An absent folder is identified by its path alone.
func (r *studioRoots) InstallationID() string {
	digest := sha256.New()
	identity, dev, ino := "", uint64(0), uint64(0)
	if r != nil {
		identity = r.identity
		if r.hasIdentity {
			dev, ino = r.dev, r.ino
		}
	}
	digest.Write([]byte(identity))
	digest.Write([]byte{0})
	digest.Write([]byte(strconv.FormatUint(dev, 10)))
	digest.Write([]byte{0})
	digest.Write([]byte(strconv.FormatUint(ino, 10)))
	return studioInstallationIDPrefix + hex.EncodeToString(digest.Sum(nil))[:32]
}

func (r *studioRoots) Installation() StudioInstallation {
	installation := StudioInstallation{ID: r.InstallationID(), Label: profileAppName, Layout: StudioLayoutPerUser}
	if r == nil {
		return installation
	}
	if r.layout == StudioLayoutPortable {
		installation.Layout = StudioLayoutPortable
	}
	if !r.installed {
		installation.Label = profileAppName + " (not installed)"
		return installation
	}
	installation.AppInstalled = true
	installation.AppVersion = profileVersion(r.version)
	if installation.AppVersion != "" {
		installation.Label = profileAppName + " " + installation.AppVersion
	}
	return installation
}

// Location is the display string Ori shows the owner, never an agent.
func (r *studioRoots) Location(spec studioSourceSpec) string {
	switch {
	case spec.Access == StudioAccessLive:
		return "REAPER (running)"
	case spec.Owner:
		home := ""
		if r != nil {
			home = r.home
		}
		return studioClipLine(studioDisplayPath(spec.Path, home), maxStudioLocationChars, spec.Label)
	case spec.Library:
		if r != nil && r.libraryDisplay != "" {
			return r.libraryDisplay
		}
		return "~/Ori Scripts/reaper"
	}
	base := "~/Library/Application Support/REAPER"
	if r != nil && r.resourceDisplay != "" {
		base = r.resourceDisplay
	}
	return base + "/" + spec.Folder
}

// FolderState says what a file source is right now: a presence look, nothing
// read (contract §4.1).
func (r *studioRoots) FolderState(spec studioSourceSpec) (state, detail string) {
	if r == nil || !r.supported {
		return StudioStateUnsupported, "Catalog sources can be read on macOS only."
	}
	if spec.Owner {
		dir, state, detail := r.OpenOwnerFolder(spec)
		closeRoot(dir)
		return state, detail
	}
	if spec.Library {
		return r.libraryState, studioFolderDetail(r.libraryState, "Ori Scripts/reaper")
	}
	if r.resourceState != StudioStateReady || r.resource == nil {
		return r.resourceState, studioFolderDetail(r.resourceState, "REAPER resource")
	}
	return studioChildFolderState(r.resource, spec.Folder)
}

// studioChildFolderState looks at one folder inside an opened real directory
// and reports the source state its contents would have. The folder is opened
// to prove it can be, then closed; nothing in it is read.
func studioChildFolderState(parent *os.Root, name string) (state, detail string) {
	info, err := parent.Lstat(name)
	switch {
	case err != nil && errors.Is(err, os.ErrNotExist):
		return StudioStateAbsent, studioFolderDetail(StudioStateAbsent, name)
	case err != nil:
		return StudioStateDenied, studioFolderDetail(StudioStateDenied, name)
	case info.Mode()&os.ModeSymlink != 0:
		return StudioStateUnsupported, "The " + name + " folder is a link, which is not followed."
	case !info.IsDir():
		return StudioStateUnsupported, "The " + name + " entry is not a folder."
	}
	dir, _, ok := openRealDir(parent, name)
	if !ok {
		return StudioStateDenied, studioFolderDetail(StudioStateDenied, name)
	}
	closeRoot(dir)
	return StudioStateReady, ""
}

func studioFolderDetail(state, name string) string {
	switch state {
	case StudioStateAbsent:
		return "There is no " + name + " folder yet."
	case StudioStateDenied:
		return "The " + name + " folder could not be opened."
	case StudioStateUnsupported:
		return "The " + name + " folder is a link, which is not followed."
	}
	return ""
}

// OpenSource opens the folder of one file source as a real directory. It is
// nil unless FolderState is ready.
func (r *studioRoots) OpenSource(spec studioSourceSpec) *os.Root {
	if r == nil || spec.Access != StudioAccessFile {
		return nil
	}
	if spec.Owner {
		dir, _, _ := r.OpenOwnerFolder(spec)
		return dir
	}
	if spec.Library {
		if r.libraryState != StudioStateReady || r.library == nil {
			return nil
		}
		return reopenRoot(r.library)
	}
	if r.resourceState != StudioStateReady || r.resource == nil {
		return nil
	}
	dir, _, ok := openRealDir(r.resource, spec.Folder)
	if !ok {
		return nil
	}
	return dir
}

// OpenOwnerFolder opens an owner folder as a real directory, one path element
// at a time from the root of the file system, and says what it is right now:
// ready (opened), absent (nothing of that name on the way), unsupported (a
// link, or not a folder) or denied (there, not openable). Nothing is created
// and no link is followed (contract §4.5).
func (r *studioRoots) OpenOwnerFolder(spec studioSourceSpec) (dir *os.Root, state, detail string) {
	if r == nil || !r.supported {
		return nil, StudioStateUnsupported, "Catalog sources can be read on macOS only."
	}
	if !spec.Owner || !studioValidFolderPath(spec.Path) {
		return nil, StudioStateUnsupported, "The folder's path is not a canonical absolute path."
	}
	top, err := os.OpenRoot(string(filepath.Separator))
	if err != nil {
		return nil, StudioStateDenied, "The folder could not be opened."
	}
	defer closeRoot(top)
	elements := strings.Split(strings.TrimPrefix(spec.Path, string(filepath.Separator)), string(filepath.Separator))
	dir, state = openStudioFolder(top, elements...)
	switch state {
	case StudioStateReady:
		return dir, state, ""
	case StudioStateAbsent:
		return nil, state, "There is no folder at this location now."
	case StudioStateUnsupported:
		return nil, state, "The folder, or a folder on the way to it, is a link or not a folder; links are not followed."
	}
	return nil, StudioStateDenied, "The folder could not be opened."
}

// openStudioFolder opens elements below parent one at a time and says what the
// last one is: ready (opened), absent (nothing of that name), unsupported (a
// link or another type) or denied (there, not openable).
func openStudioFolder(parent *os.Root, elements ...string) (*os.Root, string) {
	if parent == nil || len(elements) == 0 {
		return nil, StudioStateDenied
	}
	ancestor := openRealPath(parent, elements[:len(elements)-1]...)
	if len(elements) > 1 {
		if ancestor == nil {
			return nil, studioMissingParentState(parent, elements[:len(elements)-1])
		}
		defer closeRoot(ancestor)
	} else {
		ancestor = parent
	}
	last := elements[len(elements)-1]
	info, err := ancestor.Lstat(last)
	switch {
	case err != nil && os.IsNotExist(err):
		return nil, StudioStateAbsent
	case err != nil:
		return nil, StudioStateDenied
	case info.Mode()&os.ModeSymlink != 0, !info.IsDir():
		return nil, StudioStateUnsupported
	}
	dir, _, ok := openRealDir(ancestor, last)
	if !ok {
		return nil, StudioStateDenied
	}
	return dir, StudioStateReady
}

// studioMissingParentState tells an absent parent folder from one that could
// not be used, walking the elements again without opening the last one.
func studioMissingParentState(parent *os.Root, elements []string) string {
	current, owned := parent, false
	defer func() {
		if owned {
			closeRoot(current)
		}
	}()
	for _, element := range elements {
		info, err := current.Lstat(element)
		switch {
		case err != nil && os.IsNotExist(err):
			return StudioStateAbsent
		case err != nil:
			return StudioStateDenied
		case info.Mode()&os.ModeSymlink != 0, !info.IsDir():
			return StudioStateUnsupported
		}
		next, _, ok := openRealDir(current, element)
		if !ok {
			return StudioStateDenied
		}
		if owned {
			closeRoot(current)
		}
		current, owned = next, true
	}
	return StudioStateDenied
}

// studioDisplayPath shows a path with "~" for the home folder.
func studioDisplayPath(path, home string) string {
	if home != "" && (path == home || strings.HasPrefix(path, home+string(filepath.Separator))) {
		return "~" + filepath.ToSlash(strings.TrimPrefix(path, home))
	}
	return filepath.ToSlash(path)
}

// reopenRoot gives the caller its own handle on an already opened real
// directory, so it can be closed independently.
func reopenRoot(root *os.Root) *os.Root {
	if root == nil {
		return nil
	}
	before, err := root.Stat(".")
	if err != nil || !before.IsDir() {
		return nil
	}
	again, err := os.OpenRoot(root.Name())
	if err != nil {
		return nil
	}
	after, err := again.Stat(".")
	if err != nil || !os.SameFile(before, after) {
		_ = again.Close()
		return nil
	}
	return again
}

// StudioProbe opens the platform's view of a REAPER installation for one
// catalog read.
type StudioProbe interface {
	OpenStudio(ctx context.Context) *studioRoots
}

// StudioSources answers studio.sources.read (contract §4.1). It looks, it does
// not read: every state comes from a presence check or from the probes that
// already report REAPER's availability.
func (s *Service) StudioSources(ctx context.Context, input StudioSourcesInput) (StudioSourcesResult, error) {
	if err := validateStudioSourcesInput(input); err != nil {
		return StudioSourcesResult{}, err
	}
	roots := s.openStudio(ctx)
	defer roots.Close()
	result := StudioSourcesResult{SchemaVersion: StudioSchemaVersion, Installation: roots.Installation(), Sources: make([]StudioSource, 0, len(studioSourceSpecs)+len(input.Folders))}
	for _, spec := range studioSourceSpecs {
		source := StudioSource{ID: spec.ID, Category: spec.Category, Label: spec.Label, Location: roots.Location(spec), Access: spec.Access}
		if spec.Access == StudioAccessLive {
			source.State, source.Detail = s.studioLiveState(ctx, roots)
		} else {
			source.State, source.Detail = roots.FolderState(spec)
		}
		source.Detail = studioDetail(source.Detail)
		result.Sources = append(result.Sources, source)
	}
	// Every folder Ori sent is answered once, as it was sent, and no other
	// folder_ source exists (contract §4.5).
	for _, folder := range input.Folders {
		spec, state, detail := studioOwnerFolderSpec(folder)
		if state == "" {
			state, detail = roots.FolderState(spec)
		}
		result.Sources = append(result.Sources, StudioSource{
			ID: folder.ID, Category: folder.Category, Label: spec.Label, Location: roots.Location(spec), Access: StudioAccessFile,
			State: state, Detail: studioDetail(detail),
		})
	}
	return result, nil
}

// validateStudioSourcesInput refuses what the closed input schema refuses, and
// a folder sent twice, which no answer could report once.
func validateStudioSourcesInput(input StudioSourcesInput) error {
	invalid := errors.New("studio sources request is invalid")
	if input.SchemaVersion != StudioSchemaVersion || len(input.Folders) > maxStudioOwnerFolders {
		return invalid
	}
	seen := make(map[string]struct{}, len(input.Folders))
	for _, folder := range input.Folders {
		if _, repeated := seen[folder.ID]; repeated || !studioFolderIDShape(folder.ID) || !studioFolderRefShape(StudioFolderRef{Category: folder.Category, Path: folder.Path}) {
			return invalid
		}
		seen[folder.ID] = struct{}{}
	}
	return nil
}

// studioFolderRefShape is the closed schema's own bound on a folder: a
// category and a path of the declared lengths. Whether they are a folder this
// plugin reads is answered in the state, not refused.
func studioFolderRefShape(folder StudioFolderRef) bool {
	return folder.Category != "" && len(folder.Category) <= maxStudioSourceIDChars && folder.Path != "" && len(folder.Path) <= maxStudioFolderPathChars
}

func (s *Service) openStudio(ctx context.Context) *studioRoots {
	if s == nil || s.studio == nil {
		return &studioRoots{resourceState: StudioStateUnsupported, libraryState: StudioStateUnsupported}
	}
	roots := s.studio.OpenStudio(ctx)
	if roots == nil {
		return &studioRoots{resourceState: StudioStateUnsupported, libraryState: StudioStateUnsupported}
	}
	return roots
}

// studioLiveState is the availability ladder of the live plugin-list source:
// platform, installation, Web Remote configuration, a running REAPER, a
// registered runner, and a runner recent enough to carry the read-only
// plugin-list API (spike findings, §4.4).
func (s *Service) studioLiveState(ctx context.Context, roots *studioRoots) (state, detail string) {
	if roots == nil || !roots.supported || s == nil || s.probes.WebRemote == nil || s.probes.Transport == nil || s.probes.Runner == nil {
		return StudioStateUnsupported, "REAPER's plugin list can be read on macOS only."
	}
	if !roots.installed {
		return StudioStateAbsent, "REAPER is not installed."
	}
	web := s.probes.WebRemote.DetectWebRemote(ctx)
	switch web.State {
	case ProbeReady:
	case ProbeUnsupported:
		return StudioStateUnsupported, "REAPER's plugin list can be read on macOS only."
	default:
		return StudioStateWebRemoteOff, "Enable REAPER's Web browser interface (Preferences, Control/OSC/web) to read its plugin list."
	}
	if s.probes.Transport.CheckTransport(ctx, web).State != TransportAvailable {
		return StudioStateAppClosed, "Open REAPER to read its plugin list."
	}
	runner := s.probes.Runner.DetectRunner(ctx)
	if runner.State != ProbeReady {
		return StudioStateReaderMissing, "The Ori runner is not registered in REAPER. Set up live control once, then try again."
	}
	if runner.Version < studioReaderRunnerVersion {
		return StudioStateReaderOutdated, "The Ori runner registered in REAPER is older than this plugin. Update it from live-control setup, then run it once."
	}
	return StudioStateReady, ""
}

// StudioInventory answers studio.inventory.read (contract §4.2): one page of
// one source, cut to the request's limit and to the declared output bound.
func (s *Service) StudioInventory(ctx context.Context, input StudioInventoryInput) (StudioInventoryResult, error) {
	if err := validateStudioInventoryInput(input); err != nil {
		return StudioInventoryResult{}, err
	}
	result := StudioInventoryResult{
		SchemaVersion: StudioSchemaVersion, InstallationID: input.InstallationID, SourceID: input.SourceID,
		State: StudioStateOK, Entries: []StudioEntry{}, Coverage: StudioCoverage{Skipped: []StudioSkip{}},
	}
	roots := s.openStudio(ctx)
	defer roots.Close()
	if roots.InstallationID() != input.InstallationID {
		result.State = StudioStateInstallationChanged
		return result, nil
	}
	spec, ok := studioInventorySource(input)
	if !ok {
		result.State = StudioStateUnsupported
		return result, nil
	}
	cursor, ok := decodeStudioCursor(input.Cursor)
	if !ok {
		result.State = StudioStateCursorReset
		return result, nil
	}
	if spec.Access == StudioAccessLive {
		return s.studioLiveInventory(ctx, roots, input, cursor, result)
	}
	if state, _ := roots.FolderState(spec); state != StudioStateReady {
		result.State = state
		return result, nil
	}
	dir := roots.OpenSource(spec)
	if dir == nil {
		result.State = StudioStateDenied
		return result, nil
	}
	defer closeRoot(dir)
	facts := newStudioScriptFacts(roots, spec, dir)
	page, err := walkStudioSource(ctx, dir, spec, cursor, input.Limit, maxStudioInventoryOutputBytes-studioInventoryOutputReserve, facts)
	if err != nil {
		return StudioInventoryResult{}, err
	}
	result.Entries = page.entries
	result.NextCursor = page.nextCursor
	result.Coverage = page.coverage
	// Registration rows that point outside every source are REAPER's own
	// Scripts source's business: counted once, on its first page.
	if cursor == "" && spec.reaperScripts() && facts != nil && facts.external > 0 {
		result.Coverage.Skipped = append(result.Coverage.Skipped, StudioSkip{Reason: studioSkipExternalRegistration, Count: facts.external})
	}
	return result, nil
}

// studioInventorySource is the source a page reads. A folder_ id reads the
// owner folder named beside it, and only when that folder is the one the id
// is the digest of; any other id is one of the fixed sources and carries no
// folder (contract §4.2, §4.5). Anything else is unsupported.
func studioInventorySource(input StudioInventoryInput) (studioSourceSpec, bool) {
	if isStudioFolderSource(input.SourceID) {
		if input.Folder == nil {
			return studioSourceSpec{}, false
		}
		spec, problem, _ := studioOwnerFolderSpec(StudioFolderInput{ID: input.SourceID, Category: input.Folder.Category, Path: input.Folder.Path})
		return spec, problem == ""
	}
	if input.Folder != nil {
		return studioSourceSpec{}, false
	}
	return lookupStudioSource(input.SourceID)
}

func validateStudioInventoryInput(input StudioInventoryInput) error {
	if input.SchemaVersion != StudioSchemaVersion || len(input.InstallationID) < minStudioInstallationIDChars || len(input.InstallationID) > maxStudioInstallationIDChars ||
		!strings.HasPrefix(input.InstallationID, studioInstallationIDPrefix) || input.SourceID == "" || len(input.SourceID) > maxStudioSourceIDChars ||
		len(input.Cursor) > maxStudioCursorBytes || input.Limit < 1 || input.Limit > maxStudioPageEntries ||
		(input.Folder != nil && !studioFolderRefShape(*input.Folder)) {
		return errors.New("studio inventory request is invalid")
	}
	return nil
}

// studioDetail keeps a detail to one plain line within the declared bound, or
// drops it.
func studioDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" || !profileLine(detail, maxStudioDetailChars) {
		return ""
	}
	return detail
}

// studioEntryBytes is the size of one entry as a host counts it.
func studioEntryBytes(entry StudioEntry) (int, bool) {
	encoded, err := json.Marshal(entry)
	if err != nil {
		return 0, false
	}
	return len(encoded) + 1, true
}

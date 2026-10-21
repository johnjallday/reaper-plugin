package reaper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"unicode"
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
	studioInstallationIDPrefix   = "inst_"
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

type StudioSourcesInput struct {
	SchemaVersion int `json:"schema_version"`
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
	SchemaVersion  int    `json:"schema_version"`
	InstallationID string `json:"installation_id"`
	SourceID       string `json:"source_id"`
	Cursor         string `json:"cursor"`
	Limit          int    `json:"limit"`
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

// studioSourceSpec is one of the five fixed v1 sources (contract §4.1).
type studioSourceSpec struct {
	ID       string
	Category string
	Label    string
	Access   string
	// Folder names the sub-folder of the REAPER resource folder a file source
	// lives in. The Ori script library lives outside it (Library).
	Folder     string
	Library    bool
	Kind       string
	Extensions []string
}

var studioSourceSpecs = []studioSourceSpec{
	{ID: "project_templates", Category: StudioCategoryTemplates, Label: "Project templates", Access: StudioAccessFile,
		Folder: "ProjectTemplates", Kind: StudioKindProjectTemplate, Extensions: []string{".rpp"}},
	{ID: "track_templates", Category: StudioCategoryTemplates, Label: "Track templates", Access: StudioAccessFile,
		Folder: "TrackTemplates", Kind: StudioKindTrackTemplate, Extensions: []string{".rtracktemplate"}},
	{ID: "reaper_scripts", Category: StudioCategoryScripts, Label: "REAPER Scripts", Access: StudioAccessFile,
		Folder: "Scripts", Kind: StudioKindScript, Extensions: []string{".lua", ".eel", ".py"}},
	{ID: "ori_scripts", Category: StudioCategoryScripts, Label: "Ori script library", Access: StudioAccessFile,
		Library: true, Kind: StudioKindScript, Extensions: []string{".lua", ".eel", ".py"}},
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

// studioRoots is what the platform probe opens for one read: the installation
// facts and the two folders sources live in, each held open as a real
// directory or marked with the state a source there must report. Close
// releases the handles.
type studioRoots struct {
	supported bool
	installed bool
	version   string
	layout    string

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
	if input.SchemaVersion != StudioSchemaVersion {
		return StudioSourcesResult{}, errors.New("studio sources request is invalid")
	}
	roots := s.openStudio(ctx)
	defer roots.Close()
	result := StudioSourcesResult{SchemaVersion: StudioSchemaVersion, Installation: roots.Installation(), Sources: make([]StudioSource, 0, len(studioSourceSpecs))}
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
	return result, nil
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
	spec, ok := lookupStudioSource(input.SourceID)
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
	if cursor == "" && !spec.Library && facts != nil && facts.external > 0 {
		result.Coverage.Skipped = append(result.Coverage.Skipped, StudioSkip{Reason: studioSkipExternalRegistration, Count: facts.external})
	}
	return result, nil
}

func validateStudioInventoryInput(input StudioInventoryInput) error {
	if input.SchemaVersion != StudioSchemaVersion || len(input.InstallationID) < minStudioInstallationIDChars || len(input.InstallationID) > maxStudioInstallationIDChars ||
		!strings.HasPrefix(input.InstallationID, studioInstallationIDPrefix) || input.SourceID == "" || len(input.SourceID) > maxStudioSourceIDChars ||
		len(input.Cursor) > maxStudioCursorBytes || input.Limit < 1 || input.Limit > maxStudioPageEntries {
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

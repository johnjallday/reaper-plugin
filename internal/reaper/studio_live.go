package reaper

import (
	"context"
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

//go:embed studio_fx_inspector.lua
var canonicalStudioFXInspectorLua string

// The plugin-list source is live (spike findings, contract §4.4): REAPER's own
// EnumInstalledFX, run through the audited read-only inspector, which keeps
// transport, selection, the open project, its change count, its dirty state
// and undo history as they are. Each page re-reads the list; nothing is kept
// between calls.

const (
	maxStudioFXListBytes = 4 << 20
	maxStudioFXRows      = 20000
	maxStudioFXTextBytes = 1024
	studioFXListFileName = "fx_list.json"
	studioFXBuiltinSpace = "REAPER"
)

type studioFXRow struct {
	Name  string `json:"name"`
	Ident string `json:"ident"`
}

type studioFXList struct {
	SchemaVersion int           `json:"schema_version"`
	AppVersion    string        `json:"app_version"`
	FX            []studioFXRow `json:"fx"`
	Count         int           `json:"count"`
}

// studioFXItem is one position in the sorted plugin list: an entry, or a
// skipped row keyed right after the entry it collided with.
type studioFXItem struct {
	key   string
	entry StudioEntry
	skip  string
}

func (s *Service) studioLiveInventory(ctx context.Context, roots *studioRoots, input StudioInventoryInput, cursor string, result StudioInventoryResult) (StudioInventoryResult, error) {
	state, _ := s.studioLiveState(ctx, roots)
	if state != StudioStateReady {
		result.State = state
		return result, nil
	}
	if s.runner == nil || s.runner.roots == nil {
		result.State = StudioStateReaderMissing
		return result, nil
	}
	root, err := s.runner.roots.Resolve()
	if err != nil {
		result.State = StudioStateReaderMissing
		return result, nil
	}
	listPath := filepath.Join(root, studioFXListFileName)
	if err := removeStaleTidyRunnerFile(listPath); err != nil {
		return StudioInventoryResult{}, errors.New("studio plugin list could not be prepared")
	}
	run, runErr := s.runner.RunReadOnlyInspection(ctx, canonicalStudioFXInspectorLua)
	switch {
	case errors.Is(runErr, ErrActionDisconnected):
		result.State = StudioStateAppClosed
		return result, nil
	case errors.Is(runErr, ErrRunnerTimedOut), errors.Is(runErr, ErrRunnerUnavailable):
		// A recorded runner that does not answer is not a reader (spike: a stale
		// runner.id triggers nothing).
		result.State = StudioStateReaderMissing
		return result, nil
	case runErr != nil && strings.Contains(run.ErrorText, "read-only audit"):
		result.State = StudioStateReaderOutdated
		return result, nil
	case runErr != nil, run.Outcome != "ok":
		return StudioInventoryResult{}, errors.New("studio plugin list inspection failed")
	}
	list, err := readStudioFXList(listPath)
	if err != nil {
		return StudioInventoryResult{}, err
	}
	page := pageStudioFXList(list.FX, cursor, input.Limit, maxStudioInventoryOutputBytes-studioInventoryOutputReserve)
	result.Entries = page.entries
	result.NextCursor = page.nextCursor
	result.Coverage = page.coverage
	return result, nil
}

func readStudioFXList(path string) (studioFXList, error) {
	info, err := os.Lstat(path) // #nosec G304 -- fixed list file in the trusted runner exchange
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxStudioFXListBytes {
		return studioFXList{}, errors.New("studio plugin list is unavailable")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- bounded regular file in the trusted runner exchange
	if err != nil {
		return studioFXList{}, errors.New("studio plugin list is unavailable")
	}
	var list studioFXList
	if err := decodeStrictTidyJSON(data, maxStudioFXListBytes, &list); err != nil || list.SchemaVersion != 1 || list.Count != len(list.FX) || len(list.FX) > maxStudioFXRows {
		return studioFXList{}, errors.New("studio plugin list is invalid")
	}
	return list, nil
}

// pageStudioFXList turns the raw list into sorted items and cuts one page the
// same way the file walk does: items up to the cursor belong to earlier pages,
// skips between the cursor and the last entry are counted here, and the final
// page counts everything after its last entry.
func pageStudioFXList(rows []studioFXRow, cursor string, limit, budget int) studioPage {
	items := studioFXItems(rows)
	page := studioPage{entries: []StudioEntry{}, coverage: StudioCoverage{Skipped: []StudioSkip{}}}
	start := 0
	if cursor != "" {
		start = sort.Search(len(items), func(i int) bool { return items[i].key > cursor })
	}
	visited, skips := 0, map[string]int{}
	checkpointVisited, checkpointSkips := 0, map[string]int{}
	full := false
	for index := start; index < len(items); index++ {
		item := items[index]
		if item.skip != "" {
			visited++
			skips[item.skip]++
			continue
		}
		size, ok := studioEntryBytes(item.entry)
		if !ok {
			visited++
			skips[studioSkipUnsupportedName]++
			continue
		}
		if len(page.entries) >= limit || size > budget {
			full = true
			break
		}
		budget -= size
		visited++
		page.entries = append(page.entries, item.entry)
		checkpointVisited = visited
		checkpointSkips = map[string]int{}
		for reason, count := range skips {
			checkpointSkips[reason] = count
		}
	}
	if full {
		visited, skips = checkpointVisited, checkpointSkips
		if len(page.entries) > 0 {
			page.nextCursor = encodeStudioCursor(page.entries[len(page.entries)-1].NativeID)
		}
	}
	page.coverage.Visited = visited
	page.coverage.Skipped = studioSkipList(skips)
	return page
}

// studioFXItems maps rows to entries, drops a second row that would repeat an
// id as unsupported_name, and sorts by id.
func studioFXItems(rows []studioFXRow) []studioFXItem {
	items := make([]studioFXItem, 0, len(rows))
	seen := map[string]struct{}{}
	for _, row := range rows {
		entry, ok := studioFXEntry(row)
		if !ok {
			items = append(items, studioFXItem{key: norm.NFC.String(row.Ident), skip: studioSkipUnsupportedName})
			continue
		}
		if _, repeated := seen[entry.NativeID]; repeated {
			items = append(items, studioFXItem{key: entry.NativeID + "\x00", skip: studioSkipUnsupportedName})
			continue
		}
		seen[entry.NativeID] = struct{}{}
		items = append(items, studioFXItem{key: entry.NativeID, entry: entry})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].key < items[j].key })
	return items
}

var (
	studioFXGroupPattern = regexp.MustCompile(`\(([^()]*)\)`)
	// Groups REAPER appends for channel counts and architecture, which name no
	// vendor: "(mono)", "(64 out)", "(x86_64)", "(2->16ch)".
	studioFXDescriptorPattern = regexp.MustCompile(`^(?:\d+ out|mono|stereo|\d+ in \d+ out|x86_64|arm64|\d+->\d+ch|\d+ch)$`)
	studioFXSpaces            = regexp.MustCompile(`\s+`)
)

// studioFXEntry maps one EnumInstalledFX row to a catalog entry (spike
// findings, "Live rows"). The display name is "<PREFIX>: <name> (<vendor>)",
// the prefix ending in "i" for an instrument; the identifier is an absolute
// bundle path for VST and VST3 (with "<id>" for a shell plugin), "Vendor:
// Name" for AU, a reverse-DNS id for CLAP, a path under Effects for JS, and
// the bare name for REAPER's built-ins. The id drops the folder of a bundle
// path, so it is never absolute, and is namespaced by format.
func studioFXEntry(row studioFXRow) (StudioEntry, bool) {
	name := norm.NFC.String(strings.TrimSpace(row.Name))
	ident := norm.NFC.String(strings.TrimSpace(row.Ident))
	if name == "" || ident == "" || len(name) > maxStudioFXTextBytes || len(ident) > maxStudioFXTextBytes ||
		!utf8.ValidString(name) || !utf8.ValidString(ident) || !studioPlainText(name) || !studioPlainText(ident) || strings.Contains(ident, "..") {
		return StudioEntry{}, false
	}
	prefix, display, prefixed := strings.Cut(name, ": ")
	if !prefixed || prefix == "" || strings.ContainsAny(prefix, " ()") {
		prefix, display = "", name
	}
	format := prefix
	kind := StudioKindEffect
	if len(prefix) > 1 && strings.HasSuffix(prefix, "i") {
		format = strings.TrimSuffix(prefix, "i")
		kind = StudioKindInstrument
	}
	space := format
	if space == "" {
		space = studioFXBuiltinSpace
	}
	nativeID := space + ":" + studioFXIdentity(ident)
	if len(nativeID) > maxStudioNativeIDBytes || strings.Contains(nativeID, "..") || strings.Contains(nativeID, "/") && format != "JS" {
		return StudioEntry{}, false
	}

	vendor := ""
	for _, match := range studioFXGroupPattern.FindAllStringSubmatch(display, -1) {
		group := strings.TrimSpace(match[1])
		if group == "" || studioFXDescriptorPattern.MatchString(group) {
			continue
		}
		vendor = group
	}
	title := display
	if vendor != "" {
		title = strings.Replace(title, "("+vendor+")", "", 1)
	}
	title = strings.TrimSpace(studioFXSpaces.ReplaceAllString(title, " "))
	if !profileLine(title, maxStudioNameChars) {
		return StudioEntry{}, false
	}
	entry := StudioEntry{NativeID: nativeID, Kind: kind, Name: title, Facts: []StudioFact{}}
	if format != "" && profileLine(format, maxStudioFactValueChars) {
		entry.Facts = append(entry.Facts, StudioFact{ID: StudioFactFormat, Value: format})
	}
	if vendor != "" && profileLine(vendor, maxStudioFactValueChars) {
		entry.Facts = append(entry.Facts, StudioFact{ID: StudioFactVendor, Value: vendor})
	}
	return entry, true
}

// studioFXIdentity keeps REAPER's identifier without the folder of a bundle
// path: "/Library/Audio/Plug-Ins/VST3/Shell.vst3<123>" is "Shell.vst3<123>".
func studioFXIdentity(ident string) string {
	if !strings.HasPrefix(ident, "/") && !strings.HasPrefix(ident, "\\") {
		return ident
	}
	path, shell, hasShell := strings.Cut(ident, "<")
	path = strings.TrimRight(path, "/\\")
	if cut := strings.LastIndexAny(path, "/\\"); cut >= 0 {
		path = path[cut+1:]
	}
	if hasShell {
		return path + "<" + shell
	}
	return path
}

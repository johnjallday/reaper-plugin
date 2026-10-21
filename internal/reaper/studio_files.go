package reaper

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// File sources are listed by one deterministic depth-first walk over the
// opened source folder: children in NFC name order, a folder's contents right
// after the folder. Every page repeats the walk from the start and emits the
// entries after the cursor, so no state is kept between calls. Each item has
// one position in that order, and a page covers the items between the cursor
// and its last entry (the final page runs to the end), so a skipped item is
// counted on exactly one page and Ori can sum the counts (contract §4.2).

const (
	studioWalkDepthLimit = 16
	// studioDirectoryEntryLimit bounds how many names of one folder are
	// looked at; the rest are counted as too_large.
	studioDirectoryEntryLimit = 4096
	// studioWalkItemLimit bounds the whole walk of one call.
	studioWalkItemLimit = 100000
	studioCursorPrefix  = "after:"
)

type studioPage struct {
	entries    []StudioEntry
	nextCursor string
	coverage   StudioCoverage
}

// studioEntryDecorator adds the facts of one candidate file to its entry; it
// may read bounded metadata of that file through the opened folder.
type studioEntryDecorator interface {
	Decorate(entry *StudioEntry, dir *os.Root, name string, relative string)
}

type studioWalker struct {
	ctx       context.Context
	spec      studioSourceSpec
	cursor    string
	limit     int
	budget    int
	decorator studioEntryDecorator

	// passed says the walk has moved past the cursor; before that nothing is
	// counted or emitted.
	passed bool
	// full says the page cannot take another entry; the walk stops there.
	full    bool
	items   int
	visited int
	skips   map[string]int
	entries []StudioEntry

	// The checkpoint is the coverage at the last emitted entry. When the next
	// entry does not fit, the counts after it belong to the next page.
	checkpointVisited int
	checkpointSkips   map[string]int
}

var errStudioWalkStopped = errors.New("studio walk stopped")

func walkStudioSource(ctx context.Context, dir *os.Root, spec studioSourceSpec, cursor string, limit, budget int, decorator studioEntryDecorator) (studioPage, error) {
	if dir == nil || limit < 1 || limit > maxStudioPageEntries || budget < 1 {
		return studioPage{}, errors.New("studio walk is invalid")
	}
	walker := &studioWalker{
		ctx: ctx, spec: spec, cursor: cursor, limit: limit, budget: budget, decorator: decorator,
		passed: cursor == "", skips: map[string]int{}, checkpointSkips: map[string]int{}, entries: []StudioEntry{},
	}
	err := walker.walk(dir, "", 1)
	if err != nil && !errors.Is(err, errStudioWalkStopped) {
		return studioPage{}, err
	}
	page := studioPage{entries: walker.entries, coverage: StudioCoverage{Skipped: []StudioSkip{}}}
	if walker.full && len(walker.entries) == 0 {
		return studioPage{}, errors.New("studio entry exceeds the declared output bound")
	}
	if walker.full {
		// The page ends at its last entry; what was counted after it is counted
		// again by the next page, so it is not reported here.
		walker.visited, walker.skips = walker.checkpointVisited, walker.checkpointSkips
		if len(walker.entries) > 0 {
			page.nextCursor = encodeStudioCursor(walker.entries[len(walker.entries)-1].NativeID)
		}
	}
	page.coverage.Visited = walker.visited
	page.coverage.Skipped = studioSkipList(walker.skips)
	return page, nil
}

func studioSkipList(skips map[string]int) []StudioSkip {
	list := make([]StudioSkip, 0, len(skips))
	for reason, count := range skips {
		if count > 0 {
			list = append(list, StudioSkip{Reason: reason, Count: count})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Reason < list[j].Reason })
	return list
}

func (w *studioWalker) skip(reason string) {
	w.visited++
	w.skips[reason]++
}

func (w *studioWalker) checkpoint() {
	w.checkpointVisited = w.visited
	w.checkpointSkips = make(map[string]int, len(w.skips))
	for reason, count := range w.skips {
		w.checkpointSkips[reason] = count
	}
}

// walk visits one opened folder. prefix is the folder's path inside the
// source with a trailing slash (empty at the root); depth counts the root as 1.
func (w *studioWalker) walk(dir *os.Root, prefix string, depth int) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	names, overflow, err := readStudioDirectoryNames(dir)
	if err != nil {
		// The folder itself was opened; its listing failing is one unreadable
		// item at the folder's position, and nothing under it is reached.
		if w.passed {
			w.skip(studioSkipUnreadable)
		}
		return nil
	}
	for _, name := range names {
		if w.full {
			return errStudioWalkStopped
		}
		if err := w.visit(dir, prefix, depth, name); err != nil {
			return err
		}
	}
	if overflow > 0 && w.passed && !w.full {
		w.visited += overflow
		w.skips[studioSkipTooLarge] += overflow
	}
	return nil
}

func (w *studioWalker) visit(dir *os.Root, prefix string, depth int, name string) error {
	w.items++
	if w.items > studioWalkItemLimit {
		return errStudioWalkStopped
	}
	if w.items%64 == 0 {
		if err := w.ctx.Err(); err != nil {
			return err
		}
	}
	relative := prefix + norm.NFC.String(name)
	info, err := dir.Lstat(name)
	isDir := err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0

	// Position against the cursor: items up to and including it were listed
	// by earlier pages. A folder before the cursor is entered only when the
	// cursor lies inside it.
	if !w.passed {
		order := compareStudioPath(relative, w.cursor)
		switch {
		case order == 0:
			w.passed = true
			return nil
		case order > 0:
			w.passed = true
		case isDir && strings.HasPrefix(w.cursor, relative+"/"):
			return w.descend(dir, relative, depth, name)
		default:
			return nil
		}
	}

	switch {
	case strings.HasPrefix(name, "."):
		w.skip(studioSkipHidden)
	case err != nil:
		w.skip(studioSkipUnreadable)
	case info.Mode()&os.ModeSymlink != 0:
		w.skip(studioSkipSymlink)
	case info.IsDir():
		if depth+1 > studioWalkDepthLimit {
			w.skip(studioSkipDepthLimit)
			return nil
		}
		w.visited++
		return w.descend(dir, relative, depth, name)
	case !info.Mode().IsRegular():
		w.skip(studioSkipUnsupportedName)
	case !w.spec.matches(name):
		// A file of another type is looked at and passed over.
		w.visited++
	default:
		return w.emit(dir, name, relative, info)
	}
	return nil
}

func (w *studioWalker) descend(parent *os.Root, relative string, depth int, name string) error {
	child, _, ok := openRealDir(parent, name)
	if !ok {
		if w.passed {
			w.skips[studioSkipUnreadable]++
		}
		return nil
	}
	defer closeRoot(child)
	return w.walk(child, relative+"/", depth+1)
}

func (w *studioWalker) emit(dir *os.Root, name, relative string, info os.FileInfo) error {
	entry, ok := studioFileEntry(w.spec, name, relative, info)
	if !ok {
		w.skip(studioSkipUnsupportedName)
		return nil
	}
	if w.decorator != nil {
		w.decorator.Decorate(&entry, dir, name, relative)
	}
	size, ok := studioEntryBytes(entry)
	if !ok {
		w.skip(studioSkipUnsupportedName)
		return nil
	}
	if len(w.entries) >= w.limit || size > w.budget {
		w.full = true
		return errStudioWalkStopped
	}
	w.budget -= size
	w.visited++
	w.entries = append(w.entries, entry)
	w.checkpoint()
	return nil
}

// studioFileEntry names one candidate file. The id is its path inside the
// source in NFC with slashes; the name is the file name without its
// extension. A name that cannot be shown as one plain line, or an id over the
// bound, makes the file unsupported.
func studioFileEntry(spec studioSourceSpec, name, relative string, info os.FileInfo) (StudioEntry, bool) {
	kind, ok := spec.kindOf(name)
	if !ok || !utf8.ValidString(relative) || len(relative) > maxStudioNativeIDBytes || !studioPlainText(relative) {
		return StudioEntry{}, false
	}
	stem := strings.TrimSpace(name[:len(name)-len(filepath.Ext(name))])
	stem = norm.NFC.String(stem)
	if !profileLine(stem, maxStudioNameChars) {
		return StudioEntry{}, false
	}
	entry := StudioEntry{NativeID: relative, Kind: kind, Name: stem, Facts: []StudioFact{}}
	if modified := info.ModTime().UTC(); modified.Year() >= 1970 && modified.Year() <= 9998 {
		entry.ModifiedAt = modified.Format(time.RFC3339)
	}
	return entry, true
}

// studioPlainText refuses control characters, line and paragraph separators
// and bidirectional marks anywhere in a value a host decodes strictly.
func studioPlainText(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f || isStudioFormattingRune(r) {
			return false
		}
	}
	return true
}

func (spec studioSourceSpec) matches(name string) bool {
	_, ok := spec.kindOf(name)
	return ok
}

// readStudioDirectoryNames lists the names in an opened folder in NFC order,
// up to the per-folder bound. overflow counts the names beyond it.
func readStudioDirectoryNames(dir *os.Root) (names []string, overflow int, err error) {
	handle, err := dir.Open(".")
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = handle.Close() }()
	entries, err := handle.ReadDir(studioDirectoryEntryLimit)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, 0, err
	}
	names = make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if len(entries) == studioDirectoryEntryLimit {
		for {
			rest, readErr := handle.ReadDir(studioDirectoryEntryLimit)
			overflow += len(rest)
			if readErr != nil || len(rest) == 0 || overflow >= studioWalkItemLimit {
				break
			}
		}
	}
	sort.SliceStable(names, func(i, j int) bool {
		return norm.NFC.String(names[i]) < norm.NFC.String(names[j])
	})
	return names, overflow, nil
}

// compareStudioPath orders two source-relative paths the way the walk visits
// them: element by element, a folder before everything inside it.
func compareStudioPath(a, b string) int {
	left, right := strings.Split(a, "/"), strings.Split(b, "/")
	for i := 0; i < len(left) && i < len(right); i++ {
		if left[i] != right[i] {
			if left[i] < right[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(left) < len(right):
		return -1
	case len(left) > len(right):
		return 1
	}
	return 0
}

// A cursor is "after:" and the last entry's id, percent-encoded so it carries
// no space or control character; a slash is kept.
func encodeStudioCursor(nativeID string) string {
	var builder strings.Builder
	builder.WriteString(studioCursorPrefix)
	const hexDigits = "0123456789ABCDEF"
	for i := 0; i < len(nativeID); i++ {
		c := nativeID[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '.', c == '_', c == '~', c == '/':
			builder.WriteByte(c)
		default:
			builder.WriteByte('%')
			builder.WriteByte(hexDigits[c>>4])
			builder.WriteByte(hexDigits[c&15])
		}
	}
	return builder.String()
}

// decodeStudioCursor returns the id a cursor names, "" for the first page,
// and false for a cursor this plugin cannot honour (cursor_reset).
func decodeStudioCursor(cursor string) (string, bool) {
	if cursor == "" {
		return "", true
	}
	if len(cursor) > maxStudioCursorBytes || !strings.HasPrefix(cursor, studioCursorPrefix) {
		return "", false
	}
	encoded := cursor[len(studioCursorPrefix):]
	var decoded []byte
	for i := 0; i < len(encoded); i++ {
		c := encoded[i]
		switch {
		case c == '%':
			if i+2 >= len(encoded) {
				return "", false
			}
			high, low := unhex(encoded[i+1]), unhex(encoded[i+2])
			if high < 0 || low < 0 {
				return "", false
			}
			decoded = append(decoded, byte(high<<4|low))
			i += 2
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '.', c == '_', c == '~', c == '/':
			decoded = append(decoded, c)
		default:
			return "", false
		}
	}
	id := string(decoded)
	if id == "" || !utf8.ValidString(id) || len(id) > maxStudioNativeIDBytes || !studioPlainText(id) {
		return "", false
	}
	return id, true
}

func unhex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

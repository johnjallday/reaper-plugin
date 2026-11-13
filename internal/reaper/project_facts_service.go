package reaper

import (
	"context"
	"errors"
	"path/filepath"
	"unicode/utf8"
)

// project.facts.read reads project files the host names, read-only. It is
// called by the host itself, never offered to an agent; it needs no running
// application and never touches Web Remote. It opens each named file and
// nothing else, writes nothing and logs no path and no name.

const (
	ProjectFactsSchemaVersion = 1
	ProjectFactsFormat        = "reaper"

	maxProjectFactsFiles       = 8
	maxProjectFactsFileID      = 7
	maxProjectFactsFormatChars = 64
	maxProjectFactsPathChars   = 4096
	maxProjectFactsOutputBytes = 262144
)

// ProjectFactsInput is the request: up to eight files, each by absolute path
// and the size the host's listing saw.
type ProjectFactsInput struct {
	SchemaVersion int                    `json:"schema_version"`
	Names         bool                   `json:"names"`
	MaxFileBytes  int64                  `json:"max_file_bytes"`
	Files         []ProjectFactsFileSpec `json:"files"`
}

type ProjectFactsFileSpec struct {
	ID     int    `json:"id"`
	Format string `json:"format"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
}

// ProjectFactsResult has exactly one entry per input id, in input order.
type ProjectFactsResult struct {
	SchemaVersion int                 `json:"schema_version"`
	Files         []ProjectFactsEntry `json:"files"`
}

// ProjectFactsEntry carries only id and result unless the file was read; the
// four numbers then follow, and the four name fields only when names were
// asked for.
type ProjectFactsEntry struct {
	ID             int             `json:"id"`
	Result         string          `json:"result"`
	TempoBPM       *float64        `json:"tempo_bpm,omitempty"`
	TempoVaries    *bool           `json:"tempo_varies,omitempty"`
	LengthSeconds  *float64        `json:"length_seconds,omitempty"`
	TrackCount     *int            `json:"track_count,omitempty"`
	Tracks         []string        `json:"tracks,omitzero"`
	TracksLeftOut  *int            `json:"tracks_left_out,omitempty"`
	Markers        []ProjectMarker `json:"markers,omitzero"`
	MarkersLeftOut *int            `json:"markers_left_out,omitempty"`
}

var errProjectFactsRequest = errors.New("project facts request is invalid")

// ProjectFacts answers project.facts.read. It does not use the service's
// client, runner or any other state, so it works on a nil *Service.
func (s *Service) ProjectFacts(ctx context.Context, input ProjectFactsInput) (ProjectFactsResult, error) {
	if err := validateProjectFactsInput(input); err != nil {
		return ProjectFactsResult{}, err
	}
	result := ProjectFactsResult{SchemaVersion: ProjectFactsSchemaVersion, Files: make([]ProjectFactsEntry, 0, len(input.Files))}
	for _, file := range input.Files {
		if ctx.Err() != nil {
			result.Files = append(result.Files, ProjectFactsEntry{ID: file.ID, Result: ProjectFactsFailed})
			continue
		}
		result.Files = append(result.Files, readProjectFactsFile(file, input.MaxFileBytes, input.Names))
	}
	return result, nil
}

func validateProjectFactsInput(input ProjectFactsInput) error {
	if input.SchemaVersion != ProjectFactsSchemaVersion || input.MaxFileBytes < 1 || input.MaxFileBytes > maxProjectFactsFileBytes ||
		len(input.Files) < 1 || len(input.Files) > maxProjectFactsFiles {
		return errProjectFactsRequest
	}
	var seen [maxProjectFactsFileID + 1]bool
	for _, file := range input.Files {
		if file.ID < 0 || file.ID > maxProjectFactsFileID || seen[file.ID] ||
			utf8.RuneCountInString(file.Format) > maxProjectFactsFormatChars ||
			utf8.RuneCountInString(file.Path) > maxProjectFactsPathChars || file.Size < 0 {
			return errProjectFactsRequest
		}
		seen[file.ID] = true
	}
	return nil
}

func readProjectFactsFile(spec ProjectFactsFileSpec, maxBytes int64, names bool) ProjectFactsEntry {
	failed := ProjectFactsEntry{ID: spec.ID, Result: ProjectFactsFailed}
	if spec.Format != ProjectFactsFormat || !filepath.IsAbs(spec.Path) {
		return failed
	}
	file, err := openProjectFile(spec.Path)
	if err != nil {
		return failed
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != spec.Size {
		return failed
	}
	outcome, facts, collected := ReadProjectFacts(file, maxBytes, names)
	if outcome != ProjectFactsRead {
		return ProjectFactsEntry{ID: spec.ID, Result: outcome}
	}
	entry := ProjectFactsEntry{
		ID: spec.ID, Result: ProjectFactsRead,
		TempoBPM: &facts.TempoBPM, TempoVaries: &facts.TempoVaries, LengthSeconds: &facts.LengthSeconds, TrackCount: &facts.TrackCount,
	}
	if names && collected != nil {
		entry.Tracks, entry.TracksLeftOut = collected.Tracks, &collected.TracksLeftOut
		entry.Markers, entry.MarkersLeftOut = collected.Markers, &collected.MarkersLeftOut
	}
	return entry
}

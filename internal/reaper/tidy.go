package reaper

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

// TidySchemaVersion is the only inspected-state, edit-plan, and apply-result
// schema version accepted by the v1 Project Tidy primitives.
const TidySchemaVersion = 1

const (
	TidyVerbSetTrackColor = "set_track_color"
	TidyVerbRenameMarker  = "rename_marker"
	TidyVerbRenameRegion  = "rename_region"
	TidyVerbDeleteMarker  = "delete_marker"

	TidyApplyStatusApplied = "applied"
	TidyApplyStatusSkipped = "skipped"
	TidyApplyStatusFailed  = "failed"

	maxTidyStateBytes      = 4 << 20
	maxTidyPlanBytes       = 1 << 20
	maxTidyResultBytes     = 1 << 20
	maxTidyTracks          = 2048
	maxTidyMarkers         = 4096
	maxTidyPlanItems       = 64
	maxTidyNameBytes       = 512
	maxTidyPathBytes       = 4096
	maxTidyReasonBytes     = 1024
	maxTidyResultTextBytes = 2048
	maxTidyObjectCount     = 1_000_000
	maxTidyMarkerID        = 2_147_483_647
	maxTidyChangeCount     = 9_007_199_254_740_991 // Largest exact JSON/Lua integer.
	maxTidyPositionSeconds = 315_576_000           // Ten years.
	maxTidyFolderDepth     = 128
	maxTidyIDBytes         = 64
)

var (
	ErrInvalidTidyState  = errors.New("invalid REAPER tidy inspected state")
	ErrInvalidTidyPlan   = errors.New("invalid REAPER tidy edit plan")
	ErrInvalidTidyResult = errors.New("invalid REAPER tidy apply result")
)

// TidyRGB is the portable color representation at the JSON boundary. REAPER's
// platform-native packed integer and 0x1000000 custom-color flag never cross
// this contract; a nil *TidyRGB means the project object has no custom color.
type TidyRGB struct {
	Red   int `json:"red"`
	Green int `json:"green"`
	Blue  int `json:"blue"`
}

// TidyInspectedState is the bounded, read-only state.json contract emitted by
// the canonical inspector. Track Index and marker enumeration_index are
// zero-based presentation positions. Stable identity is Track.GUID or the
// marker tuple (IsRegion, ID), never either presentation index.
type TidyInspectedState struct {
	SchemaVersion int                   `json:"schema_version"`
	Project       TidyInspectedProject  `json:"project"`
	Tracks        []TidyInspectedTrack  `json:"tracks"`
	Markers       []TidyInspectedMarker `json:"markers"`
}

type TidyInspectedProject struct {
	Name               string `json:"name"`
	Path               string `json:"path"`
	SaveDirty          bool   `json:"save_dirty"`
	ProjectChangeCount int64  `json:"project_change_count"`
}

type TidyInspectedTrack struct {
	GUID        string   `json:"guid"`
	Index       int      `json:"index"`
	Name        string   `json:"name"`
	Color       *TidyRGB `json:"color"`
	FolderDepth int      `json:"folder_depth"`
	ItemCount   int      `json:"item_count"`
	FXCount     int      `json:"fx_count"`
}

type TidyInspectedMarker struct {
	EnumerationIndex int      `json:"enumeration_index"`
	ID               int      `json:"id"`
	IsRegion         bool     `json:"is_region"`
	PositionSeconds  float64  `json:"position_seconds"`
	EndSeconds       *float64 `json:"end_seconds,omitempty"`
	Name             string   `json:"name"`
	Color            *TidyRGB `json:"color"`
}

// TidyEditPlan is the only mutation language Project Tidy v1 accepts. The
// inspected project change count is advisory stale-state evidence; every item
// still carries and enforces its own stable target snapshot.
type TidyEditPlan struct {
	SchemaVersion    int                     `json:"schema_version"`
	PlanID           string                  `json:"plan_id"`
	InspectedProject TidyPlanProjectSnapshot `json:"inspected_project"`
	Items            []TidyPlanItem          `json:"items"`
}

type TidyPlanProjectSnapshot struct {
	Name               string `json:"name"`
	Path               string `json:"path"`
	ProjectChangeCount int64  `json:"project_change_count"`
}

type TidyPlanItem struct {
	ID      string          `json:"id"`
	Verb    string          `json:"verb"`
	Target  TidyPlanTarget  `json:"target"`
	Payload TidyPlanPayload `json:"payload"`
	Reason  string          `json:"reason"`
}

// Pointer fields distinguish a required empty snapshot name from an omitted
// field. Validation enforces the exact target/payload shape for each verb.
type TidyPlanTarget struct {
	TrackGUID               string   `json:"track_guid,omitempty"`
	MarkerID                *int     `json:"marker_id,omitempty"`
	SnapshotName            *string  `json:"snapshot_name,omitempty"`
	SnapshotPositionSeconds *float64 `json:"snapshot_position_seconds,omitempty"`
}

type TidyPlanPayload struct {
	Color            *TidyRGB `json:"color,omitempty"`
	NewName          *string  `json:"new_name,omitempty"`
	SurvivorMarkerID *int     `json:"survivor_marker_id,omitempty"`
}

// TidyApplyResult is apply_result.json. The mutation runner finalizes
// ProjectChangeCountAfter only after closing its one undo block; the canonical
// applier cannot authoritatively sample that value from inside the block.
type TidyApplyResult struct {
	SchemaVersion            int                   `json:"schema_version"`
	PlanID                   string                `json:"plan_id"`
	ProjectChangeCountBefore int64                 `json:"project_change_count_before"`
	ProjectChangeCountAfter  int64                 `json:"project_change_count_after"`
	Items                    []TidyApplyResultItem `json:"items"`
}

type TidyApplyResultItem struct {
	ID     string `json:"id"`
	Verb   string `json:"verb"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
}

func DecodeTidyInspectedState(data []byte) (TidyInspectedState, error) {
	var state TidyInspectedState
	if err := decodeStrictTidyJSON(data, maxTidyStateBytes, &state); err != nil || state.Validate() != nil {
		return TidyInspectedState{}, ErrInvalidTidyState
	}
	return state, nil
}

func DecodeTidyEditPlan(data []byte) (TidyEditPlan, error) {
	var plan TidyEditPlan
	if err := decodeStrictTidyJSON(data, maxTidyPlanBytes, &plan); err != nil || plan.Validate() != nil {
		return TidyEditPlan{}, ErrInvalidTidyPlan
	}
	return plan, nil
}

func DecodeTidyApplyResult(data []byte) (TidyApplyResult, error) {
	var result TidyApplyResult
	if err := decodeStrictTidyJSON(data, maxTidyResultBytes, &result); err != nil || result.Validate() != nil {
		return TidyApplyResult{}, ErrInvalidTidyResult
	}
	return result, nil
}

func (s TidyInspectedState) Validate() error {
	if s.SchemaVersion != TidySchemaVersion || invalidRequiredText(s.Project.Name, maxTidyNameBytes) ||
		invalidOptionalText(s.Project.Path, maxTidyPathBytes) || s.Project.ProjectChangeCount < 0 ||
		s.Project.ProjectChangeCount > maxTidyChangeCount ||
		len(s.Tracks) > maxTidyTracks || len(s.Markers) > maxTidyMarkers {
		return ErrInvalidTidyState
	}

	trackGUIDs := make(map[string]struct{}, len(s.Tracks))
	for position, track := range s.Tracks {
		if track.Index != position || !validTidyTrackGUID(track.GUID) ||
			invalidOptionalText(track.Name, maxTidyNameBytes) ||
			track.FolderDepth < -maxTidyFolderDepth || track.FolderDepth > maxTidyFolderDepth ||
			track.ItemCount < 0 || track.ItemCount > maxTidyObjectCount ||
			track.FXCount < 0 || track.FXCount > maxTidyObjectCount || !validOptionalTidyRGB(track.Color) {
			return ErrInvalidTidyState
		}
		if _, duplicate := trackGUIDs[track.GUID]; duplicate {
			return ErrInvalidTidyState
		}
		trackGUIDs[track.GUID] = struct{}{}
	}

	markerIDs := make(map[string]struct{}, len(s.Markers))
	for position, marker := range s.Markers {
		if marker.EnumerationIndex != position || marker.ID < 0 || marker.ID > maxTidyMarkerID ||
			invalidOptionalText(marker.Name, maxTidyNameBytes) || !validTidyPosition(marker.PositionSeconds) ||
			!validOptionalTidyRGB(marker.Color) {
			return ErrInvalidTidyState
		}
		if marker.IsRegion {
			if marker.EndSeconds == nil || !validTidyPosition(*marker.EndSeconds) || *marker.EndSeconds <= marker.PositionSeconds {
				return ErrInvalidTidyState
			}
		} else if marker.EndSeconds != nil {
			return ErrInvalidTidyState
		}
		identity := fmt.Sprintf("%t:%d", marker.IsRegion, marker.ID)
		if _, duplicate := markerIDs[identity]; duplicate {
			return ErrInvalidTidyState
		}
		markerIDs[identity] = struct{}{}
	}
	return nil
}

func (p TidyEditPlan) Validate() error {
	if p.SchemaVersion != TidySchemaVersion || !validTidyID(p.PlanID) ||
		invalidRequiredText(p.InspectedProject.Name, maxTidyNameBytes) ||
		invalidOptionalText(p.InspectedProject.Path, maxTidyPathBytes) ||
		p.InspectedProject.ProjectChangeCount < 0 || p.InspectedProject.ProjectChangeCount > maxTidyChangeCount ||
		len(p.Items) < 1 || len(p.Items) > maxTidyPlanItems {
		return ErrInvalidTidyPlan
	}

	itemIDs := make(map[string]struct{}, len(p.Items))
	targets := make(map[string]struct{}, len(p.Items))
	for _, item := range p.Items {
		if !validTidyID(item.ID) || invalidRequiredText(item.Reason, maxTidyReasonBytes) || item.validateShape() != nil {
			return ErrInvalidTidyPlan
		}
		if _, duplicate := itemIDs[item.ID]; duplicate {
			return ErrInvalidTidyPlan
		}
		itemIDs[item.ID] = struct{}{}
		key := item.mutationTargetKey()
		if _, duplicate := targets[key]; duplicate {
			return ErrInvalidTidyPlan
		}
		targets[key] = struct{}{}
	}
	return nil
}

func (i TidyPlanItem) validateShape() error {
	target := i.Target
	payload := i.Payload
	validMarkerTarget := func(wantPosition bool) bool {
		if target.TrackGUID != "" || target.MarkerID == nil || *target.MarkerID < 0 || *target.MarkerID > maxTidyMarkerID ||
			target.SnapshotName == nil ||
			invalidOptionalText(*target.SnapshotName, maxTidyNameBytes) {
			return false
		}
		if wantPosition {
			return target.SnapshotPositionSeconds != nil && validTidyPosition(*target.SnapshotPositionSeconds)
		}
		return target.SnapshotPositionSeconds == nil
	}

	switch i.Verb {
	case TidyVerbSetTrackColor:
		if !validTidyTrackGUID(target.TrackGUID) || target.MarkerID != nil || target.SnapshotName != nil ||
			target.SnapshotPositionSeconds != nil || payload.Color == nil || !payload.Color.valid() ||
			payload.NewName != nil || payload.SurvivorMarkerID != nil {
			return ErrInvalidTidyPlan
		}
	case TidyVerbRenameMarker, TidyVerbRenameRegion:
		if !validMarkerTarget(false) || payload.Color != nil || payload.NewName == nil ||
			invalidRequiredText(*payload.NewName, maxTidyNameBytes) || *payload.NewName == *target.SnapshotName ||
			payload.SurvivorMarkerID != nil {
			return ErrInvalidTidyPlan
		}
	case TidyVerbDeleteMarker:
		if !validMarkerTarget(true) || payload.Color != nil || payload.NewName != nil ||
			payload.SurvivorMarkerID == nil || *payload.SurvivorMarkerID < 0 ||
			*payload.SurvivorMarkerID > maxTidyMarkerID ||
			*payload.SurvivorMarkerID == *target.MarkerID {
			return ErrInvalidTidyPlan
		}
	default:
		return ErrInvalidTidyPlan
	}
	return nil
}

func (i TidyPlanItem) mutationTargetKey() string {
	if i.Verb == TidyVerbSetTrackColor {
		return "track:" + i.Target.TrackGUID
	}
	kind := "marker:"
	if i.Verb == TidyVerbRenameRegion {
		kind = "region:"
	}
	return kind + fmt.Sprint(*i.Target.MarkerID)
}

func (r TidyApplyResult) Validate() error {
	if r.SchemaVersion != TidySchemaVersion || !validTidyID(r.PlanID) ||
		r.ProjectChangeCountBefore < 0 || r.ProjectChangeCountBefore > maxTidyChangeCount ||
		r.ProjectChangeCountAfter < r.ProjectChangeCountBefore || r.ProjectChangeCountAfter > maxTidyChangeCount ||
		len(r.Items) < 1 || len(r.Items) > maxTidyPlanItems {
		return ErrInvalidTidyResult
	}
	ids := make(map[string]struct{}, len(r.Items))
	for _, item := range r.Items {
		if !validTidyID(item.ID) || !validTidyVerb(item.Verb) ||
			invalidOptionalText(item.Reason, maxTidyResultTextBytes) ||
			invalidOptionalText(item.Error, maxTidyResultTextBytes) {
			return ErrInvalidTidyResult
		}
		switch item.Status {
		case TidyApplyStatusApplied:
			if item.Reason != "" || item.Error != "" {
				return ErrInvalidTidyResult
			}
		case TidyApplyStatusSkipped:
			if invalidRequiredText(item.Reason, maxTidyResultTextBytes) || item.Error != "" {
				return ErrInvalidTidyResult
			}
		case TidyApplyStatusFailed:
			if item.Reason != "" || invalidRequiredText(item.Error, maxTidyResultTextBytes) {
				return ErrInvalidTidyResult
			}
		default:
			return ErrInvalidTidyResult
		}
		if _, duplicate := ids[item.ID]; duplicate {
			return ErrInvalidTidyResult
		}
		ids[item.ID] = struct{}{}
	}
	return nil
}

// ValidateAgainstPlan binds an otherwise valid result to the exact reviewed
// plan. Every selected plan item must produce one result in the same order,
// with the same stable ID and verb; missing, injected, or reordered rows fail.
func (r TidyApplyResult) ValidateAgainstPlan(plan TidyEditPlan) error {
	if r.Validate() != nil || plan.Validate() != nil || r.PlanID != plan.PlanID || len(r.Items) != len(plan.Items) {
		return ErrInvalidTidyResult
	}
	for index := range plan.Items {
		if r.Items[index].ID != plan.Items[index].ID || r.Items[index].Verb != plan.Items[index].Verb {
			return ErrInvalidTidyResult
		}
	}
	return nil
}

// ValidateAgainstPlanSubset binds an apply result to a non-empty, ordered
// selection from the reviewed proposal. It permits unchecked items to be absent
// while rejecting injected, duplicated, or reordered rows.
func (r TidyApplyResult) ValidateAgainstPlanSubset(plan TidyEditPlan) error {
	if r.Validate() != nil || plan.Validate() != nil || r.PlanID != plan.PlanID || len(r.Items) > len(plan.Items) {
		return ErrInvalidTidyResult
	}
	nextPlanIndex := 0
	for _, resultItem := range r.Items {
		matched := false
		for nextPlanIndex < len(plan.Items) {
			planItem := plan.Items[nextPlanIndex]
			nextPlanIndex++
			if resultItem.ID == planItem.ID && resultItem.Verb == planItem.Verb {
				matched = true
				break
			}
		}
		if !matched {
			return ErrInvalidTidyResult
		}
	}
	return nil
}

func (c TidyRGB) valid() bool {
	return c.Red >= 0 && c.Red <= 255 && c.Green >= 0 && c.Green <= 255 && c.Blue >= 0 && c.Blue <= 255
}

func validOptionalTidyRGB(color *TidyRGB) bool {
	return color == nil || color.valid()
}

func validTidyVerb(verb string) bool {
	switch verb {
	case TidyVerbSetTrackColor, TidyVerbRenameMarker, TidyVerbRenameRegion, TidyVerbDeleteMarker:
		return true
	default:
		return false
	}
}

func validTidyPosition(position float64) bool {
	return !math.IsNaN(position) && !math.IsInf(position, 0) && position >= 0 && position <= maxTidyPositionSeconds
}

func validTidyID(id string) bool {
	if len(id) < 1 || len(id) > maxTidyIDBytes || !isASCIILetterOrDigit(id[0]) {
		return false
	}
	for index := 1; index < len(id); index++ {
		char := id[index]
		if !isASCIILetterOrDigit(char) && char != '-' && char != '_' && char != '.' {
			return false
		}
	}
	return true
}

func isASCIILetterOrDigit(char byte) bool {
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9'
}

func validTidyTrackGUID(guid string) bool {
	if len(guid) != 38 || guid[0] != '{' || guid[37] != '}' {
		return false
	}
	for index := 1; index < 37; index++ {
		char := guid[index]
		if index == 9 || index == 14 || index == 19 || index == 24 {
			if char != '-' {
				return false
			}
			continue
		}
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F') {
			return false
		}
	}
	return true
}

func invalidRequiredText(value string, maxBytes int) bool {
	return strings.TrimSpace(value) == "" || invalidOptionalText(value, maxBytes)
}

func invalidOptionalText(value string, maxBytes int) bool {
	return len(value) > maxBytes || strings.ContainsRune(value, '\x00')
}

func decodeStrictTidyJSON(data []byte, maxBytes int, destination any) error {
	if len(data) == 0 || len(data) > maxBytes || destination == nil {
		return errors.New("tidy JSON size is invalid")
	}
	if err := rejectDuplicateJSONFields(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return err
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

// rejectDuplicateJSONFields closes the ambiguity left by encoding/json's
// last-key-wins behavior. The Lua decoder must enforce the same rule before
// validating a plan, so a document has one interpretation on both sides.
func rejectDuplicateJSONFields(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := walkUniqueJSONValue(decoder); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func walkUniqueJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is invalid")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON field %q", key)
			}
			seen[key] = struct{}{}
			if err := walkUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("JSON object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := walkUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("JSON array is not closed")
		}
	default:
		return errors.New("JSON delimiter is invalid")
	}
	return nil
}

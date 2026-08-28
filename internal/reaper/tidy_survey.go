package reaper

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	tidyConventionsFileName = "conventions.md"
	maxTidyConventionsBytes = 64 << 10
)

//go:embed tidy_defaults.md
var defaultTidyConventions []byte

type TidyTrackRoleConvention struct {
	Name     string
	Color    TidyRGB
	Keywords []string
}

type TidyConventions struct {
	Roles        []TidyTrackRoleConvention
	MarkerCase   string
	Numbering    string
	SectionNames []string
}

type TidyConventionsLoad struct {
	Conventions TidyConventions
	Seeded      bool
	UsedDefault bool
	Note        string
}

type TidyProposalLine struct {
	ItemID string `json:"item_id"`
	Line   string `json:"line"`
	Reason string `json:"reason"`
}

type TidySurveyProposal struct {
	ProposalID      string             `json:"proposal_id"`
	Plan            *TidyEditPlan      `json:"plan,omitempty"`
	Lines           []TidyProposalLine `json:"lines"`
	AlreadyTidy     bool               `json:"already_tidy"`
	ConventionsNote string             `json:"conventions_note,omitempty"`
}

func DefaultTidyConventions() []byte {
	return append([]byte(nil), defaultTidyConventions...)
}

// LoadTidyConventions lazily seeds the exact blueprint default for old
// workspaces. Empty, oversized, symlinked, or malformed files are not trusted;
// survey falls back to the embedded defaults and returns a visible note.
func LoadTidyConventions(projectRoot string) (TidyConventionsLoad, error) {
	root, err := secureTidyProjectRoot(projectRoot)
	if err != nil {
		return TidyConventionsLoad{}, err
	}
	defaults, err := ParseTidyConventions(defaultTidyConventions)
	if err != nil {
		return TidyConventionsLoad{}, errors.New("embedded tidy conventions are invalid")
	}
	path := filepath.Join(root, tidyConventionsFileName)
	info, statErr := os.Lstat(path) // #nosec G304 -- fixed file beneath host-resolved project root
	if errors.Is(statErr, os.ErrNotExist) {
		if err := atomicWriteTidyFile(root, path, defaultTidyConventions); err != nil {
			return TidyConventionsLoad{}, err
		}
		return TidyConventionsLoad{
			Conventions: defaults,
			Seeded:      true,
			Note:        "Created conventions.md from the Reaper Song defaults before surveying.",
		}, nil
	}
	if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maxTidyConventionsBytes {
		return TidyConventionsLoad{
			Conventions: defaults,
			UsedDefault: true,
			Note:        "conventions.md was unavailable or unsafe; used the Reaper Song defaults for this survey.",
		}, nil
	}
	data, readErr := os.ReadFile(path) // #nosec G304 -- bounded regular file beneath host-resolved project root
	if readErr != nil {
		return TidyConventionsLoad{Conventions: defaults, UsedDefault: true, Note: "conventions.md could not be read; used the Reaper Song defaults for this survey."}, nil
	}
	conventions, parseErr := ParseTidyConventions(data)
	if parseErr != nil {
		return TidyConventionsLoad{Conventions: defaults, UsedDefault: true, Note: "conventions.md was empty or unusable; used the Reaper Song defaults for this survey."}, nil
	}
	return TidyConventionsLoad{Conventions: conventions}, nil
}

func secureTidyProjectRoot(projectRoot string) (string, error) {
	root := filepath.Clean(strings.TrimSpace(projectRoot))
	if root == "." || !filepath.IsAbs(root) {
		return "", errors.New("tidy project root is invalid")
	}
	info, err := os.Lstat(root) // #nosec G304 -- host-resolved project root
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("tidy project root is invalid")
	}
	return root, nil
}

func atomicWriteTidyFile(root, destination string, data []byte) error {
	if filepath.Dir(destination) != root {
		return errors.New("tidy destination is invalid")
	}
	temporary, err := os.CreateTemp(root, ".ori-tidy-*")
	if err != nil {
		return errors.New("tidy file could not be created")
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return errors.New("tidy file could not be secured")
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return errors.New("tidy file could not be written")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return errors.New("tidy file could not be synced")
	}
	if err := temporary.Close(); err != nil {
		return errors.New("tidy file could not be closed")
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return errors.New("tidy file could not be replaced")
	}
	return nil
}

func ParseTidyConventions(data []byte) (TidyConventions, error) {
	if len(data) == 0 || len(data) > maxTidyConventionsBytes || !strings.Contains(string(data), "<!-- ori-reaper-tidy-conventions:1 -->") {
		return TidyConventions{}, errors.New("tidy conventions are invalid")
	}
	colors := make(map[string]TidyRGB)
	colorSeen := make(map[string]bool)
	keywords := make(map[string][]string)
	var markerCase, numbering string
	var sections []string
	section := ""
	inCodeBlock := false
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		switch line {
		case "## Track role colors":
			section = "colors"
			continue
		case "## Track role matching words":
			section = "keywords"
			continue
		case "## Marker and region naming":
			section = "naming"
			continue
		}
		if strings.HasPrefix(line, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if !inCodeBlock || line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "<!--") || strings.HasPrefix(line, ">") {
			continue
		}
		switch section {
		case "colors":
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			role := strings.TrimSpace(key)
			if !validTidyRole(role) || colorSeen[role] {
				return TidyConventions{}, errors.New("tidy color role is invalid")
			}
			colorSeen[role] = true
			color, err := parseTidyRGB(value)
			if err != nil {
				return TidyConventions{}, err
			}
			colors[role] = color
		case "keywords":
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			role := strings.TrimSpace(key)
			if !validTidyRole(role) || keywords[role] != nil {
				return TidyConventions{}, errors.New("tidy keyword role is invalid")
			}
			keywords[role] = splitTidyList(value)
			if len(keywords[role]) == 0 {
				return TidyConventions{}, errors.New("tidy role keywords are empty")
			}
		case "naming":
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			switch strings.TrimSpace(key) {
			case "case":
				markerCase = strings.TrimSpace(value)
			case "numbering":
				numbering = strings.TrimSpace(value)
			case "sections":
				sections = splitTidyList(value)
			default:
				return TidyConventions{}, errors.New("tidy naming key is invalid")
			}
		}
	}

	if !validTidyMarkerCase(markerCase) || (numbering != "Space Before Number" && numbering != "No Space Before Number") || len(sections) == 0 {
		return TidyConventions{}, errors.New("tidy naming rules are invalid")
	}
	roleOrder := []string{"drums", "bass", "guitars", "keys", "vocals", "bus_fx", "reference"}
	conventions := TidyConventions{MarkerCase: markerCase, Numbering: numbering, SectionNames: sections}
	for _, role := range roleOrder {
		color, colorOK := colors[role]
		words, wordsOK := keywords[role]
		if !colorOK || !wordsOK {
			return TidyConventions{}, errors.New("tidy role definition is incomplete")
		}
		conventions.Roles = append(conventions.Roles, TidyTrackRoleConvention{Name: role, Color: color, Keywords: words})
	}
	return conventions, nil
}

func validTidyRole(role string) bool {
	switch role {
	case "drums", "bass", "guitars", "keys", "vocals", "bus_fx", "reference":
		return true
	default:
		return false
	}
}

func parseTidyRGB(value string) (TidyRGB, error) {
	parts := strings.Split(value, ",")
	if len(parts) != 3 {
		return TidyRGB{}, errors.New("tidy RGB value is invalid")
	}
	channels := [3]int{}
	for index, part := range parts {
		channel, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || channel < 0 || channel > 255 {
			return TidyRGB{}, errors.New("tidy RGB value is invalid")
		}
		channels[index] = channel
	}
	return TidyRGB{Red: channels[0], Green: channels[1], Blue: channels[2]}, nil
}

func splitTidyList(value string) []string {
	var result []string
	for _, part := range strings.Split(value, ",") {
		if item := strings.TrimSpace(part); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func validTidyMarkerCase(value string) bool {
	switch value {
	case "Title Case", "Sentence case", "UPPER CASE", "lower case":
		return true
	default:
		return false
	}
}

func GenerateTidySurvey(state TidyInspectedState, conventions TidyConventions, planID, conventionsNote string) (TidySurveyProposal, error) {
	if state.Validate() != nil || !validTidyID(planID) || len(conventions.Roles) == 0 || !validTidyMarkerCase(conventions.MarkerCase) {
		return TidySurveyProposal{}, ErrInvalidTidyPlan
	}
	plan := TidyEditPlan{
		SchemaVersion: TidySchemaVersion,
		PlanID:        planID,
		InspectedProject: TidyPlanProjectSnapshot{
			Name: state.Project.Name, Path: state.Project.Path, ProjectChangeCount: state.Project.ProjectChangeCount,
		},
	}
	proposal := TidySurveyProposal{ProposalID: planID, ConventionsNote: conventionsNote}

	for _, track := range state.Tracks {
		role, matched := inferTidyTrackRole(track.Name, conventions.Roles)
		if !matched || track.Color != nil && *track.Color == role.Color {
			continue
		}
		itemID := "color-track-" + strings.ToLower(strings.ReplaceAll(strings.Trim(track.GUID, "{}"), "-", ""))
		reason := fmt.Sprintf("conventions: %s = rgb(%d, %d, %d)", role.Name, role.Color.Red, role.Color.Green, role.Color.Blue)
		item := TidyPlanItem{
			ID: itemID, Verb: TidyVerbSetTrackColor,
			Target: TidyPlanTarget{TrackGUID: track.GUID}, Payload: TidyPlanPayload{Color: &TidyRGB{Red: role.Color.Red, Green: role.Color.Green, Blue: role.Color.Blue}},
			Reason: reason,
		}
		plan.Items = append(plan.Items, item)
		current := "no color"
		if track.Color != nil {
			current = fmt.Sprintf("rgb(%d, %d, %d)", track.Color.Red, track.Color.Green, track.Color.Blue)
		}
		proposed := fmt.Sprintf("rgb(%d, %d, %d)", role.Color.Red, role.Color.Green, role.Color.Blue)
		proposal.Lines = append(proposal.Lines, TidyProposalLine{ItemID: itemID, Line: fmt.Sprintf("Track %q — %s → %s — %s", track.Name, current, proposed, reason), Reason: reason})
	}

	deleted, survivors := findExactDuplicateMarkers(state.Markers)
	for _, marker := range state.Markers {
		if deleted[marker.EnumerationIndex] {
			continue
		}
		newName, changed := canonicalTidySectionName(marker.Name, conventions)
		if !changed {
			continue
		}
		verb, kind := TidyVerbRenameMarker, "Marker"
		if marker.IsRegion {
			verb, kind = TidyVerbRenameRegion, "Region"
		}
		itemID := fmt.Sprintf("rename-%s-%d", strings.ToLower(kind), marker.ID)
		reason := fmt.Sprintf("conventions: %s and %s", conventions.MarkerCase, conventions.Numbering)
		item := TidyPlanItem{
			ID: itemID, Verb: verb,
			Target:  TidyPlanTarget{MarkerID: tidyIntPointer(marker.ID), SnapshotName: tidyStringPointer(marker.Name)},
			Payload: TidyPlanPayload{NewName: tidyStringPointer(newName)}, Reason: reason,
		}
		plan.Items = append(plan.Items, item)
		proposal.Lines = append(proposal.Lines, TidyProposalLine{ItemID: itemID, Line: fmt.Sprintf("%s %d — %q → %q — %s", kind, marker.ID, marker.Name, newName, reason), Reason: reason})
	}
	for _, marker := range state.Markers {
		if !deleted[marker.EnumerationIndex] {
			continue
		}
		survivorID := survivors[marker.EnumerationIndex]
		itemID := fmt.Sprintf("delete-marker-%d", marker.ID)
		reason := fmt.Sprintf("duplicate: exact position and same/empty name; keep marker %d", survivorID)
		item := TidyPlanItem{
			ID: itemID, Verb: TidyVerbDeleteMarker,
			Target:  TidyPlanTarget{MarkerID: tidyIntPointer(marker.ID), SnapshotName: tidyStringPointer(marker.Name), SnapshotPositionSeconds: tidyFloatPointer(marker.PositionSeconds)},
			Payload: TidyPlanPayload{SurvivorMarkerID: tidyIntPointer(survivorID)}, Reason: reason,
		}
		plan.Items = append(plan.Items, item)
		proposal.Lines = append(proposal.Lines, TidyProposalLine{ItemID: itemID, Line: fmt.Sprintf("Marker %d at %.17g — %q → delete — %s", marker.ID, marker.PositionSeconds, marker.Name, reason), Reason: reason})
	}

	if len(plan.Items) == 0 {
		proposal.AlreadyTidy = true
		proposal.Lines = []TidyProposalLine{}
		return proposal, nil
	}
	if err := plan.Validate(); err != nil {
		return TidySurveyProposal{}, err
	}
	proposal.Plan = &plan
	return proposal, nil
}

func inferTidyTrackRole(name string, roles []TidyTrackRoleConvention) (TidyTrackRoleConvention, bool) {
	normalized := " " + normalizeTidyWords(name) + " "
	var matches []TidyTrackRoleConvention
	for index := range roles {
		roleMatched := false
		for _, keyword := range roles[index].Keywords {
			candidate := normalizeTidyWords(keyword)
			if candidate != "" && strings.Contains(normalized, " "+candidate+" ") {
				roleMatched = true
				break
			}
		}
		if roleMatched {
			matches = append(matches, roles[index])
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	if len(matches) == 2 {
		if matches[0].Name == "bus_fx" {
			return matches[1], true
		}
		if matches[1].Name == "bus_fx" {
			return matches[0], true
		}
	}
	return TidyTrackRoleConvention{}, false
}

func normalizeTidyWords(value string) string {
	fields := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	return strings.Join(fields, " ")
}

func canonicalTidySectionName(name string, conventions TidyConventions) (string, bool) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", false
	}
	base := trimmed
	number := ""
	position := len(base)
	for position > 0 && base[position-1] >= '0' && base[position-1] <= '9' {
		position--
	}
	if position < len(base) {
		number = base[position:]
		base = strings.TrimSpace(strings.TrimRight(base[:position], "-_"))
	}
	normalizedBase := strings.ReplaceAll(normalizeTidyWords(base), " ", "")
	section := ""
	for _, candidate := range conventions.SectionNames {
		if strings.ReplaceAll(normalizeTidyWords(candidate), " ", "") == normalizedBase {
			section = candidate
			break
		}
	}
	if section == "" {
		return "", false
	}
	section = applyTidyCase(section, conventions.MarkerCase)
	if number != "" {
		if conventions.Numbering == "Space Before Number" {
			section += " " + number
		} else {
			section += number
		}
	}
	return section, section != name
}

func applyTidyCase(value, convention string) string {
	switch convention {
	case "UPPER CASE":
		return strings.ToUpper(value)
	case "lower case":
		return strings.ToLower(value)
	case "Sentence case":
		lower := strings.ToLower(value)
		for index, r := range lower {
			return strings.ToUpper(string(r)) + lower[index+len(string(r)):]
		}
		return lower
	default:
		words := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return r == ' ' || r == '-' })
		separators := make([]rune, 0, len(words)-1)
		for _, r := range value {
			if r == ' ' || r == '-' {
				separators = append(separators, r)
			}
		}
		for index, word := range words {
			for position, r := range word {
				words[index] = strings.ToUpper(string(r)) + word[position+len(string(r)):]
				break
			}
		}
		var result strings.Builder
		for index, word := range words {
			if index > 0 {
				result.WriteRune(separators[index-1])
			}
			result.WriteString(word)
		}
		return result.String()
	}
}

func findExactDuplicateMarkers(markers []TidyInspectedMarker) (map[int]bool, map[int]int) {
	byPosition := make(map[float64][]TidyInspectedMarker)
	for _, marker := range markers {
		if !marker.IsRegion {
			byPosition[marker.PositionSeconds] = append(byPosition[marker.PositionSeconds], marker)
		}
	}
	deleted := make(map[int]bool)
	survivors := make(map[int]int)
	for _, atPosition := range byPosition {
		sort.Slice(atPosition, func(i, j int) bool { return atPosition[i].ID < atPosition[j].ID })
		named := make(map[string][]TidyInspectedMarker)
		var empty []TidyInspectedMarker
		for _, marker := range atPosition {
			if marker.Name == "" {
				empty = append(empty, marker)
			} else {
				named[marker.Name] = append(named[marker.Name], marker)
			}
		}
		var soleNamedSurvivor *TidyInspectedMarker
		for _, group := range named {
			survivor := group[0]
			if len(named) == 1 {
				copy := survivor
				soleNamedSurvivor = &copy
			}
			for _, duplicate := range group[1:] {
				deleted[duplicate.EnumerationIndex] = true
				survivors[duplicate.EnumerationIndex] = survivor.ID
			}
		}
		if soleNamedSurvivor != nil {
			for _, duplicate := range empty {
				deleted[duplicate.EnumerationIndex] = true
				survivors[duplicate.EnumerationIndex] = soleNamedSurvivor.ID
			}
		} else if len(named) == 0 && len(empty) > 1 {
			for _, duplicate := range empty[1:] {
				deleted[duplicate.EnumerationIndex] = true
				survivors[duplicate.EnumerationIndex] = empty[0].ID
			}
		}
	}
	return deleted, survivors
}

func tidyStringPointer(value string) *string  { return &value }
func tidyIntPointer(value int) *int           { return &value }
func tidyFloatPointer(value float64) *float64 { return &value }

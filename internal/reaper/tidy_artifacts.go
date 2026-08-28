package reaper

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	tidyArtifactDirectory       = "tidy"
	maxTidyArtifactStateBytes   = 256 << 10
	maxTidyArtifactSummaryBytes = 1 << 20

	TidyProposalStatusOpen         = "open"
	TidyProposalStatusDismissed    = "dismissed"
	TidyProposalStatusSuperseded   = "superseded"
	TidyProposalStatusAlreadyTidy  = "already_tidy"
	TidyProposalStatusApplied      = "applied"
	TidyProposalStatusApplySkipped = "apply_skipped"
	TidyProposalStatusApplyFailed  = "apply_failed"
)

type TidyProposalRecord struct {
	SchemaVersion   int                `json:"schema_version"`
	ProposalID      string             `json:"proposal_id"`
	CreatedAt       time.Time          `json:"created_at"`
	Status          string             `json:"status"`
	AlreadyTidy     bool               `json:"already_tidy"`
	PlanFile        string             `json:"plan_file,omitempty"`
	SummaryFile     string             `json:"summary_file"`
	Items           []TidyProposalLine `json:"items"`
	ConventionsNote string             `json:"conventions_note,omitempty"`
	SupersededBy    string             `json:"superseded_by,omitempty"`
}

type TidyStoredProposal struct {
	Record  TidyProposalRecord
	Plan    *TidyEditPlan
	Summary string
}

type TidyArtifactStore struct {
	projectRoot string
	tidyRoot    string
}

func NewTidyArtifactStore(projectRoot string) (*TidyArtifactStore, error) {
	root, err := secureTidyProjectRoot(projectRoot)
	if err != nil {
		return nil, err
	}
	return &TidyArtifactStore{projectRoot: root, tidyRoot: filepath.Join(root, tidyArtifactDirectory)}, nil
}

// WriteProposal writes plan and Markdown first, then commits the state record
// last. Readers ignore orphaned payload files without a state record. A newer
// survey supersedes every older open record; latest selection remains
// deterministic by timestamp then immutable proposal ID even after a crash.
func (s *TidyArtifactStore) WriteProposal(proposal TidySurveyProposal, createdAt time.Time) (TidyStoredProposal, error) {
	if s == nil || !validTidyID(proposal.ProposalID) || createdAt.IsZero() || validateTidySurveyProposal(proposal) != nil {
		return TidyStoredProposal{}, errors.New("tidy proposal is invalid")
	}
	if err := s.ensureDirectory(); err != nil {
		return TidyStoredProposal{}, err
	}
	records, err := s.listRecords()
	if err != nil {
		return TidyStoredProposal{}, err
	}
	for _, existing := range records {
		if existing.ProposalID == proposal.ProposalID || !tidyProposalOrderBefore(existing.CreatedAt, existing.ProposalID, createdAt.UTC(), proposal.ProposalID) {
			return TidyStoredProposal{}, errors.New("tidy proposal is not newer than existing artifacts")
		}
	}
	for _, path := range []string{
		s.statePath(proposal.ProposalID),
		filepath.Join(s.tidyRoot, tidySummaryFileName(proposal.ProposalID)),
		filepath.Join(s.tidyRoot, tidyPlanFileName(proposal.ProposalID)),
	} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) { // #nosec G304 -- fixed proposal artifact path
			return TidyStoredProposal{}, errors.New("tidy proposal already exists")
		}
	}

	record := TidyProposalRecord{
		SchemaVersion: TidySchemaVersion,
		ProposalID:    proposal.ProposalID, CreatedAt: createdAt.UTC(),
		Status: TidyProposalStatusOpen, SummaryFile: tidySummaryFileName(proposal.ProposalID),
		Items:           append([]TidyProposalLine(nil), proposal.Lines...),
		ConventionsNote: proposal.ConventionsNote,
	}
	if proposal.AlreadyTidy {
		record.Status = TidyProposalStatusAlreadyTidy
		record.AlreadyTidy = true
	} else {
		record.PlanFile = tidyPlanFileName(proposal.ProposalID)
		planData, err := json.MarshalIndent(proposal.Plan, "", "  ")
		if err != nil || len(planData)+1 > maxTidyPlanBytes {
			return TidyStoredProposal{}, errors.New("tidy plan artifact is invalid")
		}
		if err := atomicWriteTidyFile(s.tidyRoot, filepath.Join(s.tidyRoot, record.PlanFile), append(planData, '\n')); err != nil {
			return TidyStoredProposal{}, err
		}
	}

	summary := renderTidyProposalSummary(proposal, createdAt.UTC())
	if len(summary) > maxTidyArtifactSummaryBytes {
		return TidyStoredProposal{}, errors.New("tidy summary artifact is too large")
	}
	if err := atomicWriteTidyFile(s.tidyRoot, filepath.Join(s.tidyRoot, record.SummaryFile), []byte(summary)); err != nil {
		return TidyStoredProposal{}, err
	}
	if err := s.writeRecord(record); err != nil {
		return TidyStoredProposal{}, err
	}

	for _, previous := range records {
		if previous.ProposalID == record.ProposalID || previous.Status != TidyProposalStatusOpen {
			continue
		}
		previous.Status = TidyProposalStatusSuperseded
		previous.SupersededBy = record.ProposalID
		if err := s.writeRecord(previous); err != nil {
			return TidyStoredProposal{}, err
		}
	}
	return TidyStoredProposal{Record: record, Plan: proposal.Plan, Summary: summary}, nil
}

func validateTidySurveyProposal(proposal TidySurveyProposal) error {
	if proposal.AlreadyTidy {
		if proposal.Plan != nil || len(proposal.Lines) != 0 {
			return errors.New("already-tidy proposal has changes")
		}
		return nil
	}
	if proposal.Plan == nil || proposal.Plan.PlanID != proposal.ProposalID || proposal.Plan.Validate() != nil || len(proposal.Lines) != len(proposal.Plan.Items) {
		return errors.New("tidy proposal plan is invalid")
	}
	seen := make(map[string]struct{}, len(proposal.Lines))
	for index, line := range proposal.Lines {
		if line.ItemID != proposal.Plan.Items[index].ID || invalidRequiredText(line.Line, 2048) ||
			invalidRequiredText(line.Reason, maxTidyReasonBytes) || line.Reason != proposal.Plan.Items[index].Reason {
			return errors.New("tidy proposal line is invalid")
		}
		if _, duplicate := seen[line.ItemID]; duplicate {
			return errors.New("tidy proposal line is duplicated")
		}
		seen[line.ItemID] = struct{}{}
	}
	return nil
}

func (s *TidyArtifactStore) LatestProposal() (TidyStoredProposal, bool, error) {
	records, err := s.listRecords()
	if err != nil || len(records) == 0 {
		return TidyStoredProposal{}, false, err
	}
	sort.Slice(records, func(i, j int) bool {
		return tidyProposalOrderBefore(records[i].CreatedAt, records[i].ProposalID, records[j].CreatedAt, records[j].ProposalID)
	})
	stored, err := s.loadRecordArtifacts(records[len(records)-1])
	return stored, err == nil, err
}

func (s *TidyArtifactStore) OpenProposal() (TidyStoredProposal, bool, error) {
	latest, found, err := s.LatestProposal()
	if err != nil || !found || latest.Record.Status != TidyProposalStatusOpen {
		return TidyStoredProposal{}, false, err
	}
	return latest, true, nil
}

func (s *TidyArtifactStore) ReadProposal(proposalID string) (TidyStoredProposal, error) {
	if s == nil || !validTidyID(proposalID) {
		return TidyStoredProposal{}, errors.New("tidy proposal ID is invalid")
	}
	record, err := s.readRecord(s.statePath(proposalID))
	if err != nil || record.ProposalID != proposalID {
		return TidyStoredProposal{}, errors.New("tidy proposal is unavailable")
	}
	return s.loadRecordArtifacts(record)
}

func (s *TidyArtifactStore) DismissProposal(proposalID string) error {
	stored, err := s.ReadProposal(proposalID)
	if err != nil || stored.Record.Status != TidyProposalStatusOpen {
		return errors.New("tidy proposal is not open")
	}
	stored.Record.Status = TidyProposalStatusDismissed
	return s.writeRecord(stored.Record)
}

func (s *TidyArtifactStore) ensureDirectory() error {
	info, err := os.Lstat(s.tidyRoot) // #nosec G304 -- fixed directory beneath host-resolved project root
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(s.tidyRoot, 0o750); err != nil {
			return errors.New("tidy artifact directory could not be created")
		}
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("tidy artifact directory is unsafe")
	}
	return nil
}

func (s *TidyArtifactStore) listRecords() ([]TidyProposalRecord, error) {
	if s == nil {
		return nil, errors.New("tidy artifact store is unavailable")
	}
	info, err := os.Lstat(s.tidyRoot) // #nosec G304 -- fixed directory beneath host-resolved project root
	if errors.Is(err, os.ErrNotExist) {
		return []TidyProposalRecord{}, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("tidy artifact directory is unsafe")
	}
	entries, err := os.ReadDir(s.tidyRoot) // #nosec G304 -- validated fixed artifact directory
	if err != nil {
		return nil, errors.New("tidy artifact directory is unreadable")
	}
	var records []TidyProposalRecord
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "proposal-") || !strings.HasSuffix(entry.Name(), ".state.json") {
			continue
		}
		record, err := s.readRecord(filepath.Join(s.tidyRoot, entry.Name()))
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (s *TidyArtifactStore) readRecord(path string) (TidyProposalRecord, error) {
	data, err := readBoundedRegularTidyFile(path, s.tidyRoot, maxTidyArtifactStateBytes)
	if err != nil {
		return TidyProposalRecord{}, err
	}
	var record TidyProposalRecord
	if err := decodeStrictTidyJSON(data, maxTidyArtifactStateBytes, &record); err != nil || record.Validate() != nil {
		return TidyProposalRecord{}, errors.New("tidy proposal record is invalid")
	}
	return record, nil
}

func (r TidyProposalRecord) Validate() error {
	if r.SchemaVersion != TidySchemaVersion || !validTidyID(r.ProposalID) || r.CreatedAt.IsZero() ||
		!validTidyArtifactName(r.SummaryFile, "proposal-"+r.ProposalID+".md") || len(r.Items) > maxTidyPlanItems ||
		invalidOptionalText(r.ConventionsNote, maxTidyResultTextBytes) {
		return errors.New("tidy proposal record is invalid")
	}
	switch r.Status {
	case TidyProposalStatusOpen, TidyProposalStatusDismissed, TidyProposalStatusApplied,
		TidyProposalStatusApplySkipped, TidyProposalStatusApplyFailed:
		if r.AlreadyTidy || !validTidyArtifactName(r.PlanFile, tidyPlanFileName(r.ProposalID)) || r.SupersededBy != "" {
			return errors.New("tidy proposal record status is invalid")
		}
	case TidyProposalStatusSuperseded:
		if r.AlreadyTidy || !validTidyArtifactName(r.PlanFile, tidyPlanFileName(r.ProposalID)) || !validTidyID(r.SupersededBy) || r.SupersededBy == r.ProposalID {
			return errors.New("tidy supersession is invalid")
		}
	case TidyProposalStatusAlreadyTidy:
		if !r.AlreadyTidy || r.PlanFile != "" || len(r.Items) != 0 || r.SupersededBy != "" {
			return errors.New("already-tidy record is invalid")
		}
	default:
		return errors.New("tidy proposal status is invalid")
	}
	seen := make(map[string]struct{}, len(r.Items))
	for _, item := range r.Items {
		if !validTidyID(item.ItemID) || invalidRequiredText(item.Line, 2048) || invalidRequiredText(item.Reason, maxTidyReasonBytes) {
			return errors.New("tidy proposal item is invalid")
		}
		if _, duplicate := seen[item.ItemID]; duplicate {
			return errors.New("tidy proposal item is duplicated")
		}
		seen[item.ItemID] = struct{}{}
	}
	return nil
}

func validTidyArtifactName(value, expected string) bool {
	return value == expected && filepath.Base(value) == value
}

func (s *TidyArtifactStore) loadRecordArtifacts(record TidyProposalRecord) (TidyStoredProposal, error) {
	summaryData, err := readBoundedRegularTidyFile(filepath.Join(s.tidyRoot, record.SummaryFile), s.tidyRoot, maxTidyArtifactSummaryBytes)
	if err != nil {
		return TidyStoredProposal{}, err
	}
	stored := TidyStoredProposal{Record: record, Summary: string(summaryData)}
	if record.PlanFile == "" {
		return stored, nil
	}
	planData, err := readBoundedRegularTidyFile(filepath.Join(s.tidyRoot, record.PlanFile), s.tidyRoot, maxTidyPlanBytes)
	if err != nil {
		return TidyStoredProposal{}, err
	}
	plan, err := DecodeTidyEditPlan(planData)
	if err != nil || plan.PlanID != record.ProposalID || len(plan.Items) != len(record.Items) {
		return TidyStoredProposal{}, errors.New("tidy proposal plan is invalid")
	}
	for index := range plan.Items {
		if plan.Items[index].ID != record.Items[index].ItemID || plan.Items[index].Reason != record.Items[index].Reason {
			return TidyStoredProposal{}, errors.New("tidy proposal artifacts disagree")
		}
	}
	stored.Plan = &plan
	return stored, nil
}

func readBoundedRegularTidyFile(path, root string, maximum int) ([]byte, error) {
	if filepath.Dir(path) != root {
		return nil, errors.New("tidy artifact path is invalid")
	}
	info, err := os.Lstat(path) // #nosec G304 -- exact artifact beneath validated root
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > int64(maximum) {
		return nil, errors.New("tidy artifact is invalid")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- bounded regular artifact beneath validated root
	if err != nil {
		return nil, errors.New("tidy artifact is unreadable")
	}
	return data, nil
}

func (s *TidyArtifactStore) writeRecord(record TidyProposalRecord) error {
	if record.Validate() != nil {
		return errors.New("tidy proposal record is invalid")
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil || len(data)+1 > maxTidyArtifactStateBytes {
		return errors.New("tidy proposal record is too large")
	}
	return atomicWriteTidyFile(s.tidyRoot, s.statePath(record.ProposalID), append(data, '\n'))
}

func (s *TidyArtifactStore) statePath(proposalID string) string {
	return filepath.Join(s.tidyRoot, "proposal-"+proposalID+".state.json")
}

func tidyPlanFileName(proposalID string) string {
	return "plan-" + proposalID + ".json"
}

func tidySummaryFileName(proposalID string) string {
	return "proposal-" + proposalID + ".md"
}

func tidyProposalOrderBefore(leftTime time.Time, leftID string, rightTime time.Time, rightID string) bool {
	if leftTime.Equal(rightTime) {
		return leftID < rightID
	}
	return leftTime.Before(rightTime)
}

func renderTidyProposalSummary(proposal TidySurveyProposal, createdAt time.Time) string {
	var summary strings.Builder
	summary.WriteString("# REAPER tidy-up proposal\n\n")
	summary.WriteString("Proposal: `" + proposal.ProposalID + "`  \n")
	summary.WriteString("Created: " + createdAt.Format(time.RFC3339Nano) + "\n\n")
	if proposal.ConventionsNote != "" {
		summary.WriteString("> " + strings.ReplaceAll(proposal.ConventionsNote, "\n", " ") + "\n\n")
	}
	if proposal.AlreadyTidy {
		summary.WriteString("## Project already tidy\n\nNo supported cosmetic changes were found.\n")
		return summary.String()
	}
	summary.WriteString("## Proposed changes\n\n")
	for _, line := range proposal.Lines {
		summary.WriteString(fmt.Sprintf("- [ ] **%s** — %s\n", line.ItemID, line.Line))
	}
	summary.WriteString("\nNothing in REAPER has been changed. Review the proposal before applying selected items.\n")
	return summary.String()
}

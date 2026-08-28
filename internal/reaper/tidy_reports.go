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
	TidyReportOutcomeApplied      = "applied"
	TidyReportOutcomePartial      = "partial"
	TidyReportOutcomeFullySkipped = "fully_skipped"
	TidyReportOutcomeFailed       = "failed"

	TidyUndoSentence = "All applied changes are a single undo step in REAPER (one Ctrl+Z reverts everything)."
)

type TidyApplyReportRecord struct {
	SchemaVersion            int                   `json:"schema_version"`
	ProposalID               string                `json:"proposal_id"`
	CreatedAt                time.Time             `json:"created_at"`
	Outcome                  string                `json:"outcome"`
	ResultFile               string                `json:"result_file"`
	MarkdownFile             string                `json:"markdown_file"`
	ProjectChangeCountBefore int64                 `json:"project_change_count_before"`
	ProjectChangeCountAfter  int64                 `json:"project_change_count_after"`
	Items                    []TidyApplyResultItem `json:"items"`
}

type TidyStoredApplyReport struct {
	Record   TidyApplyReportRecord
	Result   TidyApplyResult
	Markdown string
}

func (s *TidyArtifactStore) WriteApplyReport(proposalID string, result TidyApplyResult, createdAt time.Time) (TidyStoredApplyReport, error) {
	if s == nil || !validTidyID(proposalID) || createdAt.IsZero() || result.Validate() != nil || result.PlanID != proposalID {
		return TidyStoredApplyReport{}, errors.New("tidy apply report is invalid")
	}
	proposal, err := s.ReadProposal(proposalID)
	if err != nil || proposal.Record.Status != TidyProposalStatusOpen || proposal.Plan == nil || result.ValidateAgainstPlan(*proposal.Plan) != nil {
		return TidyStoredApplyReport{}, errors.New("tidy apply report does not match an open proposal")
	}
	if err := s.ensureDirectory(); err != nil {
		return TidyStoredApplyReport{}, err
	}
	record := TidyApplyReportRecord{
		SchemaVersion: TidySchemaVersion, ProposalID: proposalID, CreatedAt: createdAt.UTC(),
		Outcome: tidyReportOutcome(result.Items), ResultFile: tidyApplyResultFileName(proposalID),
		MarkdownFile:             tidyApplyReportFileName(proposalID),
		ProjectChangeCountBefore: result.ProjectChangeCountBefore,
		ProjectChangeCountAfter:  result.ProjectChangeCountAfter,
		Items:                    append([]TidyApplyResultItem(nil), result.Items...),
	}
	if record.Validate() != nil {
		return TidyStoredApplyReport{}, errors.New("tidy apply report is invalid")
	}
	for _, name := range []string{record.ResultFile, record.MarkdownFile, tidyApplyReportStateFileName(proposalID)} {
		if _, err := os.Lstat(filepath.Join(s.tidyRoot, name)); !errors.Is(err, os.ErrNotExist) { // #nosec G304 -- fixed report artifact path
			return TidyStoredApplyReport{}, errors.New("tidy apply report already exists")
		}
	}
	resultData, err := json.MarshalIndent(result, "", "  ")
	if err != nil || len(resultData)+1 > maxTidyResultBytes {
		return TidyStoredApplyReport{}, errors.New("tidy apply result is too large")
	}
	if err := atomicWriteTidyFile(s.tidyRoot, filepath.Join(s.tidyRoot, record.ResultFile), append(resultData, '\n')); err != nil {
		return TidyStoredApplyReport{}, err
	}
	markdown := renderTidyApplyReport(proposal, record)
	if len(markdown) > maxTidyArtifactSummaryBytes {
		return TidyStoredApplyReport{}, errors.New("tidy apply report is too large")
	}
	if err := atomicWriteTidyFile(s.tidyRoot, filepath.Join(s.tidyRoot, record.MarkdownFile), []byte(markdown)); err != nil {
		return TidyStoredApplyReport{}, err
	}

	switch record.Outcome {
	case TidyReportOutcomeApplied, TidyReportOutcomePartial:
		proposal.Record.Status = TidyProposalStatusApplied
	case TidyReportOutcomeFullySkipped:
		proposal.Record.Status = TidyProposalStatusApplySkipped
	default:
		proposal.Record.Status = TidyProposalStatusApplyFailed
	}
	if err := s.writeRecord(proposal.Record); err != nil {
		return TidyStoredApplyReport{}, err
	}
	if err := s.writeApplyReportRecord(record); err != nil {
		return TidyStoredApplyReport{}, err
	}
	return TidyStoredApplyReport{Record: record, Result: result, Markdown: markdown}, nil
}

func tidyReportOutcome(items []TidyApplyResultItem) string {
	applied, skipped, failed := 0, 0, 0
	for _, item := range items {
		switch item.Status {
		case TidyApplyStatusApplied:
			applied++
		case TidyApplyStatusSkipped:
			skipped++
		case TidyApplyStatusFailed:
			failed++
		}
	}
	if applied > 0 && skipped == 0 && failed == 0 {
		return TidyReportOutcomeApplied
	}
	if applied > 0 {
		return TidyReportOutcomePartial
	}
	if skipped > 0 && failed == 0 {
		return TidyReportOutcomeFullySkipped
	}
	return TidyReportOutcomeFailed
}

func (r TidyApplyReportRecord) Validate() error {
	if r.SchemaVersion != TidySchemaVersion || !validTidyID(r.ProposalID) || r.CreatedAt.IsZero() ||
		!validTidyArtifactName(r.ResultFile, tidyApplyResultFileName(r.ProposalID)) ||
		!validTidyArtifactName(r.MarkdownFile, tidyApplyReportFileName(r.ProposalID)) ||
		r.ProjectChangeCountBefore < 0 || r.ProjectChangeCountAfter < r.ProjectChangeCountBefore ||
		r.ProjectChangeCountAfter > maxTidyChangeCount || len(r.Items) < 1 || len(r.Items) > maxTidyPlanItems {
		return errors.New("tidy apply report record is invalid")
	}
	switch r.Outcome {
	case TidyReportOutcomeApplied, TidyReportOutcomePartial, TidyReportOutcomeFullySkipped, TidyReportOutcomeFailed:
	default:
		return errors.New("tidy apply report outcome is invalid")
	}
	result := TidyApplyResult{
		SchemaVersion: TidySchemaVersion, PlanID: r.ProposalID,
		ProjectChangeCountBefore: r.ProjectChangeCountBefore, ProjectChangeCountAfter: r.ProjectChangeCountAfter,
		Items: append([]TidyApplyResultItem(nil), r.Items...),
	}
	if result.Validate() != nil || tidyReportOutcome(r.Items) != r.Outcome {
		return errors.New("tidy apply report items are invalid")
	}
	return nil
}

func (s *TidyArtifactStore) LatestApplyReport() (TidyStoredApplyReport, bool, error) {
	records, err := s.listApplyReportRecords()
	if err != nil || len(records) == 0 {
		return TidyStoredApplyReport{}, false, err
	}
	sort.Slice(records, func(i, j int) bool {
		return tidyProposalOrderBefore(records[i].CreatedAt, records[i].ProposalID, records[j].CreatedAt, records[j].ProposalID)
	})
	stored, err := s.loadApplyReport(records[len(records)-1])
	return stored, err == nil, err
}

func (s *TidyArtifactStore) ReadApplyReport(proposalID string) (TidyStoredApplyReport, error) {
	if s == nil || !validTidyID(proposalID) {
		return TidyStoredApplyReport{}, errors.New("tidy apply report is unavailable")
	}
	data, err := readBoundedRegularTidyFile(filepath.Join(s.tidyRoot, tidyApplyReportStateFileName(proposalID)), s.tidyRoot, maxTidyArtifactStateBytes)
	if err != nil {
		return TidyStoredApplyReport{}, err
	}
	var record TidyApplyReportRecord
	if decodeStrictTidyJSON(data, maxTidyArtifactStateBytes, &record) != nil || record.Validate() != nil || record.ProposalID != proposalID {
		return TidyStoredApplyReport{}, errors.New("tidy apply report record is invalid")
	}
	return s.loadApplyReport(record)
}

func (s *TidyArtifactStore) listApplyReportRecords() ([]TidyApplyReportRecord, error) {
	if s == nil {
		return nil, errors.New("tidy artifact store is unavailable")
	}
	info, err := os.Lstat(s.tidyRoot) // #nosec G304 -- fixed directory beneath host-resolved project root
	if errors.Is(err, os.ErrNotExist) {
		return []TidyApplyReportRecord{}, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("tidy artifact directory is unsafe")
	}
	entries, err := os.ReadDir(s.tidyRoot) // #nosec G304 -- validated fixed artifact directory
	if err != nil {
		return nil, errors.New("tidy artifact directory is unreadable")
	}
	var records []TidyApplyReportRecord
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "report-") || !strings.HasSuffix(entry.Name(), ".state.json") {
			continue
		}
		data, err := readBoundedRegularTidyFile(filepath.Join(s.tidyRoot, entry.Name()), s.tidyRoot, maxTidyArtifactStateBytes)
		if err != nil {
			return nil, err
		}
		var record TidyApplyReportRecord
		if decodeStrictTidyJSON(data, maxTidyArtifactStateBytes, &record) != nil || record.Validate() != nil {
			return nil, errors.New("tidy apply report record is invalid")
		}
		records = append(records, record)
	}
	return records, nil
}

func (s *TidyArtifactStore) loadApplyReport(record TidyApplyReportRecord) (TidyStoredApplyReport, error) {
	resultData, err := readBoundedRegularTidyFile(filepath.Join(s.tidyRoot, record.ResultFile), s.tidyRoot, maxTidyResultBytes)
	if err != nil {
		return TidyStoredApplyReport{}, err
	}
	result, err := DecodeTidyApplyResult(resultData)
	if err != nil || result.PlanID != record.ProposalID || len(result.Items) != len(record.Items) {
		return TidyStoredApplyReport{}, errors.New("tidy apply result is invalid")
	}
	for index := range result.Items {
		if result.Items[index] != record.Items[index] {
			return TidyStoredApplyReport{}, errors.New("tidy apply report artifacts disagree")
		}
	}
	markdown, err := readBoundedRegularTidyFile(filepath.Join(s.tidyRoot, record.MarkdownFile), s.tidyRoot, maxTidyArtifactSummaryBytes)
	if err != nil || !strings.Contains(string(markdown), TidyUndoSentence) {
		return TidyStoredApplyReport{}, errors.New("tidy apply report Markdown is invalid")
	}
	return TidyStoredApplyReport{Record: record, Result: result, Markdown: string(markdown)}, nil
}

func (s *TidyArtifactStore) writeApplyReportRecord(record TidyApplyReportRecord) error {
	if record.Validate() != nil {
		return errors.New("tidy apply report record is invalid")
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil || len(data)+1 > maxTidyArtifactStateBytes {
		return errors.New("tidy apply report record is too large")
	}
	return atomicWriteTidyFile(s.tidyRoot, filepath.Join(s.tidyRoot, tidyApplyReportStateFileName(record.ProposalID)), append(data, '\n'))
}

func renderTidyApplyReport(proposal TidyStoredProposal, record TidyApplyReportRecord) string {
	lines := make(map[string]TidyProposalLine, len(proposal.Record.Items))
	for _, item := range proposal.Record.Items {
		lines[item.ItemID] = item
	}
	var report strings.Builder
	report.WriteString("# REAPER tidy-up report\n\n")
	report.WriteString("Proposal: `" + record.ProposalID + "`  \n")
	report.WriteString("Outcome: **" + strings.ReplaceAll(record.Outcome, "_", " ") + "**  \n")
	report.WriteString(fmt.Sprintf("Project change count: %d → %d\n\n", record.ProjectChangeCountBefore, record.ProjectChangeCountAfter))
	for _, status := range []string{TidyApplyStatusFailed, TidyApplyStatusSkipped, TidyApplyStatusApplied} {
		title := strings.ToUpper(status[:1]) + status[1:]
		report.WriteString("## " + title + "\n\n")
		count := 0
		for _, item := range record.Items {
			if item.Status != status {
				continue
			}
			count++
			line := lines[item.ID].Line
			if line == "" {
				line = item.ID
			}
			detail := item.Reason
			if item.Status == TidyApplyStatusFailed {
				detail = item.Error
			}
			report.WriteString("- " + line)
			if detail != "" {
				report.WriteString(" — " + detail)
			}
			report.WriteString("\n")
		}
		if count == 0 {
			report.WriteString("- None\n")
		}
		report.WriteString("\n")
	}
	if record.Outcome == TidyReportOutcomeFullySkipped {
		report.WriteString("Run a fresh survey before trying to apply this cleanup again.\n\n")
	}
	report.WriteString(TidyUndoSentence + "\n")
	return report.String()
}

func tidyApplyResultFileName(proposalID string) string {
	return "apply-result-" + proposalID + ".json"
}

func tidyApplyReportFileName(proposalID string) string {
	return "report-" + proposalID + ".md"
}

func tidyApplyReportStateFileName(proposalID string) string {
	return "report-" + proposalID + ".state.json"
}

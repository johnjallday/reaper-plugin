package reaper

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TidyProposalReadInput struct {
	ProposalID string `json:"proposal_id,omitempty"`
}

type TidyProposalActionInput struct {
	ProposalID string `json:"proposal_id"`
}

type TidySurfaceStatus struct {
	State             string `json:"state"`
	ProposalID        string `json:"proposal_id,omitempty"`
	HasOpenProposal   bool   `json:"has_open_proposal"`
	ItemCount         int    `json:"item_count"`
	AlreadyTidy       bool   `json:"already_tidy"`
	UpdatedAt         string `json:"updated_at,omitempty"`
	LastReportOutcome string `json:"last_report_outcome,omitempty"`
}

type TidyProposalView struct {
	ProposalID      string               `json:"proposal_id,omitempty"`
	State           string               `json:"state"`
	HasOpenProposal bool                 `json:"has_open_proposal"`
	AlreadyTidy     bool                 `json:"already_tidy"`
	CreatedAt       string               `json:"created_at,omitempty"`
	ConventionsNote string               `json:"conventions_note,omitempty"`
	Items           []TidyProposalLine   `json:"items"`
	LastReport      *TidyApplyReportView `json:"last_report,omitempty"`
}

type TidyApplyReportView struct {
	ProposalID               string                `json:"proposal_id"`
	Outcome                  string                `json:"outcome"`
	CreatedAt                string                `json:"created_at"`
	ProjectChangeCountBefore int64                 `json:"project_change_count_before"`
	ProjectChangeCountAfter  int64                 `json:"project_change_count_after"`
	Items                    []TidyApplyResultItem `json:"items"`
	FreshSurveyRecommended   bool                  `json:"fresh_survey_recommended"`
	UndoSummary              string                `json:"undo_summary"`
}

func (s *Service) TidyStation(ctx context.Context, host HostContext) StationResult {
	live := s.Station(ctx, host)
	if live.State != "ready" {
		return live
	}
	status, err := s.TidyStatus(host)
	if err != nil {
		return StationResult{State: "degraded", Value: "Proposal unavailable", Description: "Project Tidy artifacts could not be read.", CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	}
	switch status.State {
	case TidyProposalStatusOpen:
		live.Value = "1 proposal"
		live.Description = "A tidy-up proposal is ready to review."
	case TidyProposalStatusAlreadyTidy:
		live.Value = "Project tidy"
		live.Description = "The latest survey found no supported cosmetic changes."
	case TidyProposalStatusApplied:
		live.Value = "Tidy complete"
		live.Description = "The latest approved tidy plan has been applied."
	case TidyProposalStatusApplySkipped:
		live.Value = "Fresh survey needed"
		live.Description = "Every selected item was stale and skipped."
	case TidyProposalStatusApplyFailed:
		live.Value = "Review tidy report"
		live.Description = "The latest tidy apply reported failures."
	default:
		live.Value = "Ready"
		live.Description = "Survey this project for cosmetic cleanup."
	}
	return live
}

func (s *Service) TidyStatus(host HostContext) (TidySurfaceStatus, error) {
	if s == nil {
		return TidySurfaceStatus{}, errors.New("tidy status is unavailable")
	}
	store, err := tidyStoreForHost(host)
	if err != nil {
		return TidySurfaceStatus{}, errors.New("tidy status is unavailable")
	}
	latest, found, err := store.LatestProposal()
	if err != nil {
		return TidySurfaceStatus{}, errors.New("tidy status is unavailable")
	}
	if !found {
		return TidySurfaceStatus{State: "none"}, nil
	}
	status := TidySurfaceStatus{
		State: latest.Record.Status, ProposalID: latest.Record.ProposalID,
		HasOpenProposal: latest.Record.Status == TidyProposalStatusOpen,
		ItemCount:       len(latest.Record.Items), AlreadyTidy: latest.Record.AlreadyTidy,
		UpdatedAt: latest.Record.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if report, reportFound, reportErr := store.LatestApplyReport(); reportErr != nil {
		return TidySurfaceStatus{}, errors.New("tidy status is unavailable")
	} else if reportFound {
		status.LastReportOutcome = report.Record.Outcome
	}
	return status, nil
}

func (s *Service) ReadTidyProposal(host HostContext, input TidyProposalReadInput) (TidyProposalView, error) {
	if s == nil {
		return TidyProposalView{}, errors.New("tidy proposal is unavailable")
	}
	store, err := tidyStoreForHost(host)
	if err != nil {
		return TidyProposalView{}, errors.New("tidy proposal is unavailable")
	}
	var stored TidyStoredProposal
	if strings.TrimSpace(input.ProposalID) == "" {
		var found bool
		stored, found, err = store.LatestProposal()
		if err == nil && !found {
			return TidyProposalView{State: "none", Items: []TidyProposalLine{}}, nil
		}
	} else {
		if !validTidyID(input.ProposalID) {
			return TidyProposalView{}, errors.New("tidy proposal is unavailable")
		}
		stored, err = store.ReadProposal(input.ProposalID)
	}
	if err != nil {
		return TidyProposalView{}, errors.New("tidy proposal is unavailable")
	}
	view := TidyProposalView{
		ProposalID: stored.Record.ProposalID, State: stored.Record.Status,
		HasOpenProposal: stored.Record.Status == TidyProposalStatusOpen,
		AlreadyTidy:     stored.Record.AlreadyTidy,
		CreatedAt:       stored.Record.CreatedAt.UTC().Format(time.RFC3339Nano),
		ConventionsNote: stored.Record.ConventionsNote,
		Items:           append([]TidyProposalLine(nil), stored.Record.Items...),
	}
	if report, reportFound, reportErr := store.LatestApplyReport(); reportErr != nil {
		return TidyProposalView{}, errors.New("tidy proposal is unavailable")
	} else if reportFound {
		projection := tidyApplyReportProjection(report)
		view.LastReport = &projection
	}
	return view, nil
}

func tidyApplyReportProjection(report TidyStoredApplyReport) TidyApplyReportView {
	return TidyApplyReportView{
		ProposalID: report.Record.ProposalID, Outcome: report.Record.Outcome,
		CreatedAt:                report.Record.CreatedAt.UTC().Format(time.RFC3339Nano),
		ProjectChangeCountBefore: report.Record.ProjectChangeCountBefore,
		ProjectChangeCountAfter:  report.Record.ProjectChangeCountAfter,
		Items:                    append([]TidyApplyResultItem(nil), report.Record.Items...),
		FreshSurveyRecommended:   report.Record.Outcome == TidyReportOutcomeFullySkipped,
		UndoSummary:              TidyUndoSentence,
	}
}

func (s *Service) DismissTidyProposal(host HostContext, input TidyProposalActionInput) (OperationResult, error) {
	if s == nil || !validTidyID(input.ProposalID) {
		return OperationResult{}, errors.New("tidy proposal is unavailable")
	}
	store, err := tidyStoreForHost(host)
	if err != nil || store.DismissProposal(input.ProposalID) != nil {
		return OperationResult{}, errors.New("tidy proposal is unavailable")
	}
	return OperationResult{Outcome: "dismissed", Summary: "Tidy-up proposal dismissed without changing REAPER."}, nil
}

func tidyStoreForHost(host HostContext) (*TidyArtifactStore, error) {
	projectRoot, err := resolveTidyProjectRoot(host)
	if err != nil {
		return nil, err
	}
	return NewTidyArtifactStore(projectRoot)
}

func resolveTidyProjectRoot(host HostContext) (string, error) {
	if strings.TrimSpace(host.WorkspaceID) == "" || !validHostProject(host.ProjectEntry) {
		return "", errors.New("tidy host context is invalid")
	}
	projectEntry := filepath.Clean(host.ProjectEntry)
	entryInfo, err := os.Lstat(projectEntry) // #nosec G304 -- host-injected authoritative project entry
	if err != nil || !entryInfo.Mode().IsRegular() || entryInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("tidy project entry is invalid")
	}
	projectRoot := filepath.Dir(projectEntry)
	rootInfo, err := os.Lstat(projectRoot) // #nosec G304 -- parent of host-injected authoritative project entry
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("tidy project root is invalid")
	}
	if strings.TrimSpace(host.WorkspaceRoot) != "" {
		workspaceRoot := filepath.Clean(host.WorkspaceRoot)
		if !filepath.IsAbs(workspaceRoot) {
			return "", errors.New("tidy workspace root is invalid")
		}
		workspaceInfo, err := os.Lstat(workspaceRoot) // #nosec G304 -- host-injected authorized workspace root
		if err != nil || !workspaceInfo.IsDir() || workspaceInfo.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("tidy workspace root is invalid")
		}
		relative, err := filepath.Rel(workspaceRoot, projectRoot)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", errors.New("tidy project is outside the workspace")
		}
	}
	return projectRoot, nil
}

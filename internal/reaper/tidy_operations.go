package reaper

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

//go:embed tidy_inspector.lua
var canonicalTidyInspectorLua string

type TidySurveyResult struct {
	Outcome            string `json:"outcome"`
	Summary            string `json:"summary"`
	ProposalID         string `json:"proposal_id"`
	ItemCount          int    `json:"item_count"`
	AlreadyTidy        bool   `json:"already_tidy"`
	ProjectChangeCount int64  `json:"project_change_count"`
}

// RunTidySurvey is the capability-scoped survey entry point for native CLI
// agents. It keeps arbitrary localhost access disabled: the trusted plugin
// service owns the exact canonical inspector, validates its output, confirms
// the open project, and persists the deterministic proposal artifacts.
func (s *Service) RunTidySurvey(ctx context.Context, host HostContext) (TidySurveyResult, error) {
	if s == nil || s.runner == nil || s.manager == nil {
		return TidySurveyResult{}, errors.New("tidy survey is unavailable")
	}
	projectRoot, err := resolveTidyProjectRoot(host)
	if err != nil {
		return TidySurveyResult{}, errors.New("tidy survey is unavailable")
	}

	statePath := filepath.Join(s.manager.OriDir(), "state.json")
	if err := removeStaleTidyInspection(statePath); err != nil {
		return TidySurveyResult{}, err
	}
	if result, runErr := s.runner.RunReadOnlyInspection(ctx, canonicalTidyInspectorLua); runErr != nil || result.Outcome != "ok" {
		return TidySurveyResult{}, errors.New("tidy survey inspection failed")
	}
	state, err := readTidyInspection(statePath)
	if err != nil {
		return TidySurveyResult{}, err
	}
	if !sameProjectPath(state.Project.Path, host.ProjectEntry) {
		return TidySurveyResult{}, errors.New("tidy survey project does not match the workspace")
	}

	conventions, err := LoadTidyConventions(projectRoot)
	if err != nil {
		return TidySurveyResult{}, errors.New("tidy survey conventions are unavailable")
	}
	proposalID, err := opaquePlanID()
	if err != nil {
		return TidySurveyResult{}, errors.New("tidy survey could not create a proposal")
	}
	proposal, err := GenerateTidySurvey(state, conventions.Conventions, proposalID, conventions.Note)
	if err != nil {
		return TidySurveyResult{}, errors.New("tidy survey proposal is invalid")
	}
	store, err := NewTidyArtifactStore(projectRoot)
	if err != nil {
		return TidySurveyResult{}, errors.New("tidy survey artifacts are unavailable")
	}
	stored, err := store.WriteProposal(proposal, time.Now().UTC())
	if err != nil {
		return TidySurveyResult{}, errors.New("tidy survey artifacts could not be written")
	}

	outcome := "proposal"
	summary := fmt.Sprintf("Created a cosmetic-only tidy proposal with %d item(s).", len(stored.Record.Items))
	if proposal.AlreadyTidy {
		outcome = "already_tidy"
		summary = "Project already tidy."
	}
	return TidySurveyResult{
		Outcome: outcome, Summary: summary, ProposalID: proposalID,
		ItemCount: len(stored.Record.Items), AlreadyTidy: proposal.AlreadyTidy,
		ProjectChangeCount: state.Project.ProjectChangeCount,
	}, nil
}

func removeStaleTidyInspection(path string) error {
	info, err := os.Lstat(path) // #nosec G304 -- fixed state file in the trusted runner exchange
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("tidy survey state path is unsafe")
	}
	if err := os.Remove(path); err != nil { // #nosec G304 -- validated fixed state file in the trusted runner exchange
		return errors.New("tidy survey could not clear stale state")
	}
	return nil
}

func readTidyInspection(path string) (TidyInspectedState, error) {
	info, err := os.Lstat(path) // #nosec G304 -- fixed state file in the trusted runner exchange
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxTidyStateBytes {
		return TidyInspectedState{}, errors.New("tidy survey state is unavailable")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- bounded regular file in the trusted runner exchange
	if err != nil {
		return TidyInspectedState{}, errors.New("tidy survey state is unavailable")
	}
	state, err := DecodeTidyInspectedState(data)
	if err != nil {
		return TidyInspectedState{}, errors.New("tidy survey state is invalid")
	}
	return state, nil
}

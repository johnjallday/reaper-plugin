package reaper

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

//go:embed tidy_inspector.lua
var canonicalTidyInspectorLua string

//go:embed tidy_applier.lua
var canonicalTidyApplierLua string

type TidySurveyResult struct {
	Outcome            string `json:"outcome"`
	Summary            string `json:"summary"`
	ProposalID         string `json:"proposal_id"`
	ItemCount          int    `json:"item_count"`
	AlreadyTidy        bool   `json:"already_tidy"`
	ProjectChangeCount int64  `json:"project_change_count"`
}

type TidyApplySelectionInput struct {
	ProposalID      string   `json:"proposal_id"`
	SelectedItemIDs []string `json:"selected_item_ids"`
}

type TidyApplyOperationResult struct {
	ProposalID               string                `json:"proposal_id"`
	Outcome                  string                `json:"outcome"`
	Summary                  string                `json:"summary"`
	ProjectChangeCountBefore int64                 `json:"project_change_count_before"`
	ProjectChangeCountAfter  int64                 `json:"project_change_count_after"`
	Items                    []TidyApplyResultItem `json:"items"`
	UndoSummary              string                `json:"undo_summary"`
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

	state, err := s.runTidyInspection(ctx, host)
	if err != nil {
		return TidySurveyResult{}, err
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

// RunTidyApplySelection applies only the checked rows from one open proposal.
// The plugin service owns the runner files and canonical applier so a scoped CLI
// agent never receives arbitrary localhost or home-directory authority.
func (s *Service) RunTidyApplySelection(ctx context.Context, host HostContext, input TidyApplySelectionInput) (TidyApplyOperationResult, error) {
	if s == nil || s.runner == nil || s.manager == nil || !validTidyID(input.ProposalID) || len(input.SelectedItemIDs) < 1 || len(input.SelectedItemIDs) > maxTidyPlanItems {
		return TidyApplyOperationResult{}, errors.New("tidy apply selection is invalid")
	}
	projectRoot, err := resolveTidyProjectRoot(host)
	if err != nil {
		return TidyApplyOperationResult{}, errors.New("tidy apply is unavailable")
	}
	store, err := NewTidyArtifactStore(projectRoot)
	if err != nil {
		return TidyApplyOperationResult{}, errors.New("tidy apply artifacts are unavailable")
	}
	proposal, err := store.ReadProposal(input.ProposalID)
	if err != nil || proposal.Record.Status != TidyProposalStatusOpen || proposal.Plan == nil {
		return TidyApplyOperationResult{}, errors.New("tidy apply proposal is unavailable")
	}
	selectedPlan, err := selectedTidyPlan(*proposal.Plan, input.SelectedItemIDs)
	if err != nil {
		return TidyApplyOperationResult{}, err
	}
	if _, err := s.runTidyInspection(ctx, host); err != nil {
		return TidyApplyOperationResult{}, err
	}

	planData, err := json.MarshalIndent(selectedPlan, "", "  ")
	if err != nil || len(planData)+1 > maxTidyPlanBytes {
		return TidyApplyOperationResult{}, errors.New("tidy apply plan is invalid")
	}
	runnerRoot := s.manager.OriDir()
	if err := writeTidyRunnerPlan(runnerRoot, append(planData, '\n')); err != nil {
		return TidyApplyOperationResult{}, err
	}
	resultPath := filepath.Join(runnerRoot, "apply_result.json")
	if err := removeStaleTidyRunnerFile(resultPath); err != nil {
		return TidyApplyOperationResult{}, err
	}
	if result, runErr := s.runner.RunTidyApplier(ctx, canonicalTidyApplierLua); runErr != nil || result.Outcome != "ok" {
		return TidyApplyOperationResult{}, errors.New("tidy apply runner failed")
	}
	applyResult, err := readTidyApplyResult(resultPath)
	if err != nil || applyResult.ValidateAgainstPlan(selectedPlan) != nil {
		return TidyApplyOperationResult{}, errors.New("tidy apply result is invalid")
	}
	report, err := store.WriteApplyReport(input.ProposalID, applyResult, time.Now().UTC())
	if err != nil {
		return TidyApplyOperationResult{}, errors.New("tidy apply report could not be written")
	}
	summary := fmt.Sprintf("Project Tidy apply finished with outcome %s.", report.Record.Outcome)
	return TidyApplyOperationResult{
		ProposalID: input.ProposalID, Outcome: report.Record.Outcome, Summary: summary,
		ProjectChangeCountBefore: applyResult.ProjectChangeCountBefore,
		ProjectChangeCountAfter:  applyResult.ProjectChangeCountAfter,
		Items:                    append([]TidyApplyResultItem(nil), applyResult.Items...), UndoSummary: TidyUndoSentence,
	}, nil
}

func (s *Service) runTidyInspection(ctx context.Context, host HostContext) (TidyInspectedState, error) {
	statePath := filepath.Join(s.manager.OriDir(), "state.json")
	if err := removeStaleTidyRunnerFile(statePath); err != nil {
		return TidyInspectedState{}, err
	}
	if result, runErr := s.runner.RunReadOnlyInspection(ctx, canonicalTidyInspectorLua); runErr != nil || result.Outcome != "ok" {
		return TidyInspectedState{}, errors.New("tidy survey inspection failed")
	}
	state, err := readTidyInspection(statePath)
	if err != nil {
		return TidyInspectedState{}, err
	}
	if !sameProjectPath(state.Project.Path, host.ProjectEntry) {
		return TidyInspectedState{}, errors.New("tidy survey project does not match the workspace")
	}
	return state, nil
}

func selectedTidyPlan(plan TidyEditPlan, selectedIDs []string) (TidyEditPlan, error) {
	selected := make(map[string]bool, len(selectedIDs))
	for _, id := range selectedIDs {
		if !validTidyID(id) || selected[id] {
			return TidyEditPlan{}, errors.New("tidy apply selection is invalid")
		}
		selected[id] = true
	}
	result := plan
	result.Items = nil
	for _, item := range plan.Items {
		if selected[item.ID] {
			result.Items = append(result.Items, item)
		}
	}
	if len(result.Items) != len(selected) || result.Validate() != nil {
		return TidyEditPlan{}, errors.New("tidy apply selection is invalid")
	}
	return result, nil
}

func writeTidyRunnerPlan(root string, data []byte) error {
	info, err := os.Lstat(root) // #nosec G304 -- trusted runner root resolved by the service
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("tidy apply runner root is unsafe")
	}
	if err := atomicWriteTidyFile(root, filepath.Join(root, "plan.json"), data); err != nil {
		return errors.New("tidy apply plan could not be written")
	}
	return nil
}

func removeStaleTidyRunnerFile(path string) error {
	info, err := os.Lstat(path) // #nosec G304 -- fixed output file in the trusted runner exchange
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("tidy runner output path is unsafe")
	}
	if err := os.Remove(path); err != nil { // #nosec G304 -- validated fixed output file in the trusted runner exchange
		return errors.New("tidy runner could not clear stale output")
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

func readTidyApplyResult(path string) (TidyApplyResult, error) {
	info, err := os.Lstat(path) // #nosec G304 -- fixed result file in the trusted runner exchange
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxTidyResultBytes {
		return TidyApplyResult{}, errors.New("tidy apply result is unavailable")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- bounded regular file in the trusted runner exchange
	if err != nil {
		return TidyApplyResult{}, errors.New("tidy apply result is unavailable")
	}
	result, err := DecodeTidyApplyResult(data)
	if err != nil {
		return TidyApplyResult{}, errors.New("tidy apply result is invalid")
	}
	return result, nil
}

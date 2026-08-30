package reaper

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

const ServiceProtocolVersion = 1

type Service struct {
	manager   *Manager
	client    *Client
	catalog   *Catalog
	library   *Library
	runner    *Runner
	runtime   *RuntimeProvider
	mu        sync.Mutex
	last      OperationResult
	plans     map[string]PendingPlan
	undos     map[string]TrackEdit
	proposals map[string]ScriptProposal
}

func NewService(manager *Manager, probes ProbeSet, roots RunnerRootResolver) *Service {
	if manager == nil {
		manager = NewManagerFromEnv()
	}
	client := NewClient(probes)
	library := NewLibrary()
	catalog := NewCatalog()
	catalog.SetLibrary(library)
	return &Service{
		manager: manager, client: client, catalog: catalog, library: library,
		runner: NewRunner(roots, probes, client), runtime: NewRuntimeProvider(manager, probes),
		plans: make(map[string]PendingPlan), undos: make(map[string]TrackEdit), proposals: make(map[string]ScriptProposal),
	}
}

type Envelope[T any] struct {
	ProtocolVersion int         `json:"protocol_version"`
	OperationID     string      `json:"operation_id"`
	Context         HostContext `json:"context"`
	Input           T           `json:"input"`
}

type EmptyInput struct{}

type ServiceInfo struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	ProtocolVersion int    `json:"protocol_version"`
	Healthy         bool   `json:"healthy"`
}

type StationResult struct {
	State       string `json:"state"`
	Value       string `json:"value"`
	Description string `json:"description"`
	CheckedAt   string `json:"checked_at"`
}

type ActionInput struct {
	ActionID string `json:"action_id"`
}

type ScriptIDInput struct {
	ID string `json:"id"`
}

type ScriptWriteInput struct {
	ID                string `json:"id,omitempty"`
	Filename          string `json:"filename,omitempty"`
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	NeedsConfirmation bool   `json:"needs_confirmation"`
	Code              string `json:"code"`
}

type DraftInput struct {
	Code string `json:"code"`
}

type DraftValidation struct {
	Valid   bool   `json:"valid"`
	Summary string `json:"summary"`
}

type OperationResult struct {
	Outcome string `json:"outcome"`
	Summary string `json:"summary"`
}

type ProposalInput struct {
	ProposalID        string `json:"proposal_id,omitempty"`
	Filename          string `json:"filename,omitempty"`
	Name              string `json:"name,omitempty"`
	Description       string `json:"description,omitempty"`
	Code              string `json:"code,omitempty"`
	NeedsConfirmation bool   `json:"needs_confirmation,omitempty"`
}

type ScriptProposal struct {
	ID                string    `json:"id"`
	WorkspaceID       string    `json:"-"`
	Filename          string    `json:"filename"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	Code              string    `json:"code"`
	NeedsConfirmation bool      `json:"needs_confirmation"`
	Tested            bool      `json:"tested"`
	TestSummary       string    `json:"test_summary,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type ProposalResult struct {
	Proposal *ScriptProposal `json:"proposal,omitempty"`
	Outcome  string          `json:"outcome"`
	Summary  string          `json:"summary"`
}

type TrackEditInput struct {
	Edit TrackEdit `json:"edit"`
}

type TrackEditResult struct {
	Outcome       string `json:"outcome"`
	Summary       string `json:"summary"`
	State         State  `json:"state"`
	UndoAvailable bool   `json:"undo_available"`
}

type PlanInput struct {
	PlanID string      `json:"plan_id,omitempty"`
	Edits  []TrackEdit `json:"edits,omitempty"`
}

type PendingPlan struct {
	ID          string      `json:"id"`
	WorkspaceID string      `json:"-"`
	Edits       []TrackEdit `json:"edits"`
	CreatedAt   time.Time   `json:"created_at"`
	ExpiresAt   time.Time   `json:"expires_at"`
}

type PlanResult struct {
	Plan    *PendingPlan `json:"plan,omitempty"`
	Outcome string       `json:"outcome"`
	Summary string       `json:"summary"`
}

func (s *Service) Runtime() *RuntimeProvider {
	if s == nil {
		return nil
	}
	return s.runtime
}

func (s *Service) Info() ServiceInfo {
	return ServiceInfo{Name: "reaper-plugin", Version: "0.4.1", ProtocolVersion: ServiceProtocolVersion, Healthy: s != nil}
}

func (s *Service) Station(ctx context.Context, host HostContext) StationResult {
	checked := time.Now().UTC().Format(time.RFC3339)
	if s == nil || s.client == nil {
		return StationResult{State: "unavailable", Value: "REAPER unavailable", Description: "The REAPER provider is unavailable.", CheckedAt: checked}
	}
	state, err := s.readState(ctx, host)
	if err != nil {
		return StationResult{State: "degraded", Value: "State unavailable", Description: "REAPER state could not be read.", CheckedAt: checked}
	}
	if !state.Connected {
		return StationResult{State: "attention", Value: "REAPER offline", Description: "Open REAPER and check Web Remote setup.", CheckedAt: checked}
	}
	value := state.Project
	if state.Tempo > 0 {
		value = strings.TrimSpace(value + " · " + formatTempo(state.Tempo) + " BPM")
	}
	return StationResult{State: "ready", Value: value, Description: "REAPER live control is connected.", CheckedAt: checked}
}

func formatTempo(tempo float64) string {
	text := strings.TrimRight(strings.TrimRight(strconvFormatFloat(tempo), "0"), ".")
	if text == "" {
		return "0"
	}
	return text
}

func strconvFormatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func (s *Service) ReadState(ctx context.Context, host HostContext) (State, error) {
	return s.readState(ctx, host)
}

func (s *Service) readState(ctx context.Context, host HostContext) (State, error) {
	if s == nil || s.client == nil {
		return State{}, ErrClientUnavailable
	}
	if !validHostProject(host.ProjectEntry) {
		return s.client.Connected(ctx), nil
	}
	state, err := s.client.ReadState(ctx, ProjectSource{Path: host.ProjectEntry, EntryPath: host.ProjectEntry})
	if err == nil && s.runner != nil {
		state.TrackEditingAvailable = s.runner.Available(ctx)
	}
	return state, err
}

func (s *Service) Actions() ([]Action, error) {
	if s == nil || s.catalog == nil {
		return nil, errors.New("action catalog unavailable")
	}
	actions, err := s.catalog.List()
	if len(actions) > 256 {
		actions = actions[:256]
	}
	return actions, err
}

func (s *Service) RunAction(ctx context.Context, host HostContext, input ActionInput, confirmed bool) (OperationResult, error) {
	if s == nil || s.catalog == nil || s.client == nil {
		return OperationResult{}, errors.New("action unavailable")
	}
	action, found, err := s.catalog.Find(input.ActionID)
	if err != nil || !found {
		return OperationResult{}, errors.New("action unavailable")
	}
	requiresConfirmation := action.ResolveTier() == TierConfirm
	if requiresConfirmation != confirmed {
		return OperationResult{}, errors.New("action policy mismatch")
	}
	var summary string
	if action.Source == ActionSourceCustom {
		script, err := s.library.Read(action.ID)
		if err != nil {
			return OperationResult{}, errors.New("custom action unavailable")
		}
		result, err := s.runner.RunScript(ctx, script.Code)
		if err != nil {
			return OperationResult{}, errors.New("runner action failed")
		}
		summary = result.Outcome
	} else {
		if _, err := s.client.RunAction(ctx, action.ID, ProjectSource{Path: host.ProjectEntry, EntryPath: host.ProjectEntry}); err != nil {
			return OperationResult{}, errors.New("REAPER action failed")
		}
		summary = action.Label
	}
	result := OperationResult{Outcome: "ok", Summary: summary}
	s.remember(result)
	return result, nil
}

func (s *Service) RunRawAction(ctx context.Context, host HostContext, input ActionInput) (OperationResult, error) {
	if s == nil || s.client == nil || !ValidRawCommandID(input.ActionID) {
		return OperationResult{}, errors.New("raw REAPER command is invalid")
	}
	if _, err := s.client.RunAction(ctx, input.ActionID, ProjectSource{Path: host.ProjectEntry, EntryPath: host.ProjectEntry}); err != nil {
		return OperationResult{}, errors.New("raw REAPER command failed")
	}
	result := OperationResult{Outcome: "ok", Summary: "Raw REAPER command completed."}
	s.remember(result)
	return result, nil
}

func (s *Service) Scripts() ([]Script, error) {
	scripts, err := s.library.List()
	if len(scripts) > 256 {
		scripts = scripts[:256]
	}
	return scripts, err
}
func (s *Service) ReadScript(input ScriptIDInput) (Script, error) { return s.library.Read(input.ID) }
func (s *Service) CreateScript(input ScriptWriteInput) (Script, error) {
	return s.library.Create(ScriptInput{Filename: input.Filename, Name: input.Name, Description: input.Description, NeedsConfirmation: input.NeedsConfirmation, Code: input.Code})
}
func (s *Service) UpdateScript(input ScriptWriteInput) (Script, error) {
	return s.library.Update(input.ID, ScriptInput{Name: input.Name, Description: input.Description, NeedsConfirmation: input.NeedsConfirmation, Code: input.Code})
}
func (s *Service) DeleteScript(input ScriptIDInput) (OperationResult, error) {
	if err := s.library.Delete(input.ID); err != nil {
		return OperationResult{}, err
	}
	return OperationResult{Outcome: "ok", Summary: "Script deleted."}, nil
}

func (s *Service) ValidateDraft(input DraftInput) DraftValidation {
	code := strings.TrimSpace(input.Code)
	if code == "" || len(code) > maxRunnerScriptBytes {
		return DraftValidation{Summary: "Lua draft must contain at most 1 MiB."}
	}
	return DraftValidation{Valid: true, Summary: "Lua draft is within the runner limit."}
}

func (s *Service) RunDraft(ctx context.Context, input DraftInput) (OperationResult, error) {
	if !s.ValidateDraft(input).Valid || s.runner == nil {
		return OperationResult{}, errors.New("draft is invalid")
	}
	result, err := s.runner.RunScript(ctx, input.Code)
	if err != nil {
		return OperationResult{}, errors.New("draft execution failed")
	}
	output := OperationResult{Outcome: result.Outcome, Summary: "Draft executed through the registered runner."}
	s.remember(output)
	return output, nil
}

func (s *Service) ProposeScript(host HostContext, input ProposalInput) (ProposalResult, error) {
	if host.WorkspaceID == "" || ValidateScriptInput(ScriptInput{
		Filename: input.Filename, Name: input.Name, Description: input.Description,
		NeedsConfirmation: input.NeedsConfirmation, Code: input.Code,
	}) != nil {
		return ProposalResult{}, errors.New("script proposal is invalid")
	}
	id, err := opaquePlanID()
	if err != nil {
		return ProposalResult{}, errors.New("script proposal is unavailable")
	}
	proposal := ScriptProposal{
		ID: id, WorkspaceID: host.WorkspaceID, Filename: input.Filename,
		Name: input.Name, Description: input.Description, Code: input.Code,
		NeedsConfirmation: input.NeedsConfirmation, CreatedAt: time.Now().UTC(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proposals == nil {
		s.proposals = make(map[string]ScriptProposal)
	}
	if len(s.proposals) >= 256 {
		return ProposalResult{}, errors.New("script proposal capacity reached")
	}
	s.proposals[id] = proposal
	copy := proposal
	copy.WorkspaceID = ""
	return ProposalResult{Proposal: &copy, Outcome: "pending", Summary: "Review and test this script proposal."}, nil
}

func (s *Service) CurrentProposal(host HostContext) ProposalResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	var current *ScriptProposal
	for _, proposal := range s.proposals {
		if proposal.WorkspaceID != host.WorkspaceID || current != nil && !proposal.CreatedAt.After(current.CreatedAt) {
			continue
		}
		copy := proposal
		copy.WorkspaceID = ""
		current = &copy
	}
	if current == nil {
		return ProposalResult{Outcome: "none", Summary: "No script proposal is pending."}
	}
	return ProposalResult{Proposal: current, Outcome: "pending", Summary: "Review and test this script proposal."}
}

func (s *Service) ReadProposal(host HostContext, input ProposalInput) (ProposalResult, error) {
	proposal, err := s.proposal(host, input.ProposalID)
	if err != nil {
		return ProposalResult{}, err
	}
	proposal.WorkspaceID = ""
	return ProposalResult{Proposal: &proposal, Outcome: "pending", Summary: "Review and test this script proposal."}, nil
}

func (s *Service) TestProposal(ctx context.Context, host HostContext, input ProposalInput) (ProposalResult, error) {
	proposal, err := s.proposal(host, input.ProposalID)
	if err != nil || s.runner == nil {
		return ProposalResult{}, errors.New("script proposal is unavailable")
	}
	result, err := s.runner.RunScript(ctx, proposal.Code)
	if err != nil {
		return ProposalResult{}, errors.New("script proposal test failed")
	}
	proposal.Tested = result.Outcome == "ok"
	proposal.TestSummary = "Script test completed through the registered runner."
	s.mu.Lock()
	s.proposals[proposal.ID] = proposal
	s.mu.Unlock()
	copy := proposal
	copy.WorkspaceID = ""
	return ProposalResult{Proposal: &copy, Outcome: "tested", Summary: proposal.TestSummary}, nil
}

func (s *Service) SaveProposal(host HostContext, input ProposalInput) (ProposalResult, error) {
	proposal, err := s.proposal(host, input.ProposalID)
	if err != nil || !proposal.Tested {
		return ProposalResult{}, errors.New("script proposal must be tested before global save")
	}
	if _, err := s.library.Create(ScriptInput{
		Filename: proposal.Filename, Name: proposal.Name, Description: proposal.Description,
		NeedsConfirmation: proposal.NeedsConfirmation, Code: proposal.Code,
	}); err != nil {
		return ProposalResult{}, errors.New("script proposal could not be saved")
	}
	s.mu.Lock()
	delete(s.proposals, proposal.ID)
	s.mu.Unlock()
	return ProposalResult{Outcome: "saved", Summary: "Tested script saved to the global REAPER library."}, nil
}

func (s *Service) DiscardProposal(host HostContext, input ProposalInput) (ProposalResult, error) {
	proposal, err := s.proposal(host, input.ProposalID)
	if err != nil {
		return ProposalResult{}, err
	}
	s.mu.Lock()
	delete(s.proposals, proposal.ID)
	s.mu.Unlock()
	return ProposalResult{Outcome: "discarded", Summary: "Script proposal discarded without writing a library file."}, nil
}

func (s *Service) proposal(host HostContext, id string) (ScriptProposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	proposal, ok := s.proposals[strings.TrimSpace(id)]
	if !ok || proposal.WorkspaceID != host.WorkspaceID {
		return ScriptProposal{}, errors.New("script proposal is unavailable")
	}
	return proposal, nil
}

func (s *Service) RunTrackEdit(ctx context.Context, host HostContext, input TrackEditInput) (TrackEditResult, error) {
	if s == nil || s.runner == nil || input.Edit.Validate() != nil {
		return TrackEditResult{}, errors.New("track edit is invalid")
	}
	if input.Edit.Kind == TrackEditMove {
		state, err := s.readState(ctx, host)
		if err != nil || ValidateTrackMove(state, input.Edit) != nil {
			return TrackEditResult{}, errors.New("track move preflight failed")
		}
	}
	receipt, err := s.runner.RunTrackEdit(ctx, input.Edit)
	if err != nil {
		return TrackEditResult{}, errors.New("track edit execution failed")
	}
	if !receipt.Applied {
		state, stateErr := s.readState(ctx, host)
		if stateErr != nil {
			return TrackEditResult{}, errors.New("track edit result is unavailable")
		}
		s.mu.Lock()
		_, undoAvailable := s.undos[host.WorkspaceID]
		s.mu.Unlock()
		return TrackEditResult{
			Outcome: receipt.Refusal, Summary: "REAPER changed before the edit. Nothing was applied.",
			State: state, UndoAvailable: undoAvailable,
		}, nil
	}
	s.mu.Lock()
	if s.undos == nil {
		s.undos = make(map[string]TrackEdit)
	}
	s.undos[host.WorkspaceID] = input.Edit.Inverse(receipt.Prior)
	s.mu.Unlock()
	state, stateErr := s.readState(ctx, host)
	if stateErr != nil {
		return TrackEditResult{}, errors.New("track edit result is unavailable")
	}
	return TrackEditResult{Outcome: "applied", Summary: "REAPER track edit applied.", State: state, UndoAvailable: true}, nil
}

func (s *Service) UndoTrackEdit(ctx context.Context, host HostContext) (TrackEditResult, error) {
	if s == nil || s.runner == nil {
		return TrackEditResult{}, errors.New("track undo is unavailable")
	}
	s.mu.Lock()
	inverse, ok := s.undos[host.WorkspaceID]
	if ok {
		delete(s.undos, host.WorkspaceID)
	}
	s.mu.Unlock()
	if !ok {
		return TrackEditResult{}, errors.New("track undo is unavailable")
	}
	receipt, err := s.runner.RunTrackEdit(ctx, inverse)
	if err != nil || !receipt.Applied {
		return TrackEditResult{}, errors.New("track undo failed")
	}
	state, err := s.readState(ctx, host)
	if err != nil {
		return TrackEditResult{}, errors.New("track undo result is unavailable")
	}
	return TrackEditResult{Outcome: "undone", Summary: "The latest REAPER track edit was undone.", State: state}, nil
}

func (s *Service) ProposePlan(host HostContext, input PlanInput) (PlanResult, error) {
	plan := BulkPlan{Edits: append([]TrackEdit(nil), input.Edits...)}
	if host.WorkspaceID == "" || len(plan.Edits) > 64 || plan.Validate() != nil {
		return PlanResult{}, errors.New("track plan is invalid")
	}
	id, err := opaquePlanID()
	if err != nil {
		return PlanResult{}, errors.New("track plan is unavailable")
	}
	now := time.Now().UTC()
	pending := PendingPlan{
		ID: id, WorkspaceID: host.WorkspaceID, Edits: plan.Edits,
		CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, existing := range s.plans {
		if time.Now().After(existing.ExpiresAt) {
			delete(s.plans, key)
		}
	}
	if len(s.plans) >= 256 {
		return PlanResult{}, errors.New("track plan capacity reached")
	}
	s.plans[id] = pending
	copy := pending
	copy.WorkspaceID = ""
	return PlanResult{Plan: &copy, Outcome: "pending", Summary: "Review this REAPER track plan before applying it."}, nil
}

func (s *Service) CurrentPlan(host HostContext) PlanResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	var current *PendingPlan
	for id, plan := range s.plans {
		if time.Now().After(plan.ExpiresAt) {
			delete(s.plans, id)
			continue
		}
		if plan.WorkspaceID != host.WorkspaceID || current != nil && !plan.CreatedAt.After(current.CreatedAt) {
			continue
		}
		copy := plan
		copy.WorkspaceID = ""
		copy.Edits = append([]TrackEdit(nil), plan.Edits...)
		current = &copy
	}
	if current == nil {
		return PlanResult{Outcome: "none", Summary: "No REAPER track plan is pending."}
	}
	return PlanResult{Plan: current, Outcome: "pending", Summary: "Review this REAPER track plan before applying it."}
}

func (s *Service) ReadPlan(host HostContext, input PlanInput) (PlanResult, error) {
	plan, err := s.pendingPlan(host, input.PlanID)
	if err != nil {
		return PlanResult{}, err
	}
	plan.WorkspaceID = ""
	return PlanResult{Plan: &plan, Outcome: "pending", Summary: "REAPER track plan is waiting for review."}, nil
}

func (s *Service) ApplyPlan(ctx context.Context, host HostContext, input PlanInput) (PlanResult, error) {
	plan, err := s.pendingPlan(host, input.PlanID)
	if err != nil || s.runner == nil {
		return PlanResult{}, errors.New("track plan is unavailable")
	}
	receipt, err := s.runner.RunBulkPlan(ctx, BulkPlan{Edits: plan.Edits})
	if err != nil {
		return PlanResult{}, errors.New("track plan execution failed")
	}
	if !receipt.Applied {
		return PlanResult{Outcome: receipt.Refusal, Summary: "REAPER changed since this plan was proposed. Nothing was applied."}, nil
	}
	s.mu.Lock()
	delete(s.plans, plan.ID)
	s.mu.Unlock()
	result := PlanResult{Outcome: "applied", Summary: "REAPER applied the reviewed track plan as one undo step."}
	s.remember(OperationResult{Outcome: result.Outcome, Summary: result.Summary})
	return result, nil
}

func (s *Service) CancelPlan(host HostContext, input PlanInput) (PlanResult, error) {
	plan, err := s.pendingPlan(host, input.PlanID)
	if err != nil {
		return PlanResult{}, err
	}
	s.mu.Lock()
	delete(s.plans, plan.ID)
	s.mu.Unlock()
	return PlanResult{Outcome: "cancelled", Summary: "REAPER track plan cancelled. Nothing was applied."}, nil
}

func (s *Service) pendingPlan(host HostContext, id string) (PendingPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	plan, ok := s.plans[strings.TrimSpace(id)]
	if !ok || plan.WorkspaceID != host.WorkspaceID || time.Now().After(plan.ExpiresAt) {
		if ok && time.Now().After(plan.ExpiresAt) {
			delete(s.plans, plan.ID)
		}
		return PendingPlan{}, errors.New("track plan is unavailable")
	}
	plan.Edits = append([]TrackEdit(nil), plan.Edits...)
	return plan, nil
}

func opaquePlanID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func (s *Service) LastResult() OperationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

func (s *Service) remember(result OperationResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = result
}

package reaper

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

const ServiceProtocolVersion = 1

type Service struct {
	manager *Manager
	client  *Client
	catalog *Catalog
	library *Library
	runner  *Runner
	runtime *RuntimeProvider
	mu      sync.Mutex
	last    OperationResult
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

func (s *Service) Runtime() *RuntimeProvider {
	if s == nil {
		return nil
	}
	return s.runtime
}

func (s *Service) Info() ServiceInfo {
	return ServiceInfo{Name: "reaper-plugin", Version: "0.3.0", ProtocolVersion: ServiceProtocolVersion, Healthy: s != nil}
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

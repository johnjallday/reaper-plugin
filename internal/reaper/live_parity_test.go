package reaper

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLiveDisposableParity is the opt-in deletion-gate smoke against a project
// the operator has explicitly designated disposable. It never opens a project;
// the caller must point ORI_REAPER_LIVE_PROJECT at the currently open .rpp.
func TestLiveDisposableParity(t *testing.T) {
	project := os.Getenv("ORI_REAPER_LIVE_PROJECT")
	if project == "" {
		t.Skip("set ORI_REAPER_LIVE_PROJECT to the currently open disposable .rpp")
	}
	if err := ApplyServiceHomeOverride(); err != nil {
		t.Fatal(err)
	}
	roots := NewRunnerRootResolver()
	probes := NewPlatformProbeSet(roots)
	service := NewService(nil, probes, roots)
	host := HostContext{WorkspaceID: "live-parity", ProjectEntry: project}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	state, err := service.ReadState(ctx, host)
	if err != nil || !state.Connected || len(state.Tracks) == 0 || !state.TrackEditingAvailable {
		t.Fatalf("live state = %+v, %v", state, err)
	}
	actions, err := service.Actions()
	if err != nil || len(actions) < 9 {
		t.Fatalf("live action catalog = %d, %v", len(actions), err)
	}
	hasRegistered := false
	for _, action := range actions {
		hasRegistered = hasRegistered || action.Source == ActionSourceRegistered
	}
	if !hasRegistered {
		t.Fatal("live registered ReaScript catalog is empty")
	}
	if _, err := service.RunRawAction(ctx, host, ActionInput{ActionID: "not-a-command"}); err == nil {
		t.Fatal("invalid raw command reached REAPER")
	}
	assertNoSensitiveLiveText(t, state.Reason)
	original := state.Tracks[len(state.Tracks)-1]
	name := fmt.Sprintf("Ori parity guarded track %d", time.Now().UnixNano())

	// A single guarded edit stores one specific inverse, and a stale identity is
	// refused without consuming it.
	renamed, err := service.RunTrackEdit(ctx, host, TrackEditInput{Edit: RenameEdit(original.Index, original.Name, name)})
	if err != nil || renamed.Outcome != "applied" || !renamed.UndoAvailable {
		t.Fatalf("rename = %+v, %v", renamed, err)
	}
	stale, err := service.RunTrackEdit(ctx, host, TrackEditInput{Edit: MuteEdit(original.Index, original.Name, true)})
	if err != nil || stale.Outcome == "applied" {
		t.Fatalf("stale track identity result = %+v, %v", stale, err)
	}
	undone, err := service.UndoTrackEdit(ctx, host)
	if err != nil || undone.Outcome != "undone" {
		t.Fatalf("specific undo = %+v, %v", undone, err)
	}

	for label, edit := range map[string]TrackEdit{
		"color": ColorEdit(original.Index, original.Name, 0x0144cc88),
		"mute":  MuteEdit(original.Index, original.Name, !original.Muted),
		"solo":  SoloEdit(original.Index, original.Name, !original.Soloed),
		"arm":   ArmEdit(original.Index, original.Name, !original.Armed),
	} {
		result, editErr := service.RunTrackEdit(ctx, host, TrackEditInput{Edit: edit})
		if editErr != nil || result.Outcome != "applied" {
			t.Fatalf("%s edit = %+v, %v", label, result, editErr)
		}
		if result, undoErr := service.UndoTrackEdit(ctx, host); undoErr != nil || result.Outcome != "undone" {
			t.Fatalf("%s undo = %+v, %v", label, result, undoErr)
		}
	}
	if original.Index > 1 && !original.IsFolderParent() && state.FolderDepthAvailable {
		moved, moveErr := service.RunTrackEdit(ctx, host, TrackEditInput{Edit: MoveEdit(original.Index, original.Name, original.Index-1)})
		if moveErr != nil || moved.Outcome != "applied" {
			t.Fatalf("move edit = %+v, %v", moved, moveErr)
		}
		if result, undoErr := service.UndoTrackEdit(ctx, host); undoErr != nil || result.Outcome != "undone" {
			t.Fatalf("move undo = %+v, %v", result, undoErr)
		}
	}

	// A plan is inert until explicit apply, then one global Undo restores it.
	planName := "Ori parity planned track"
	proposed, err := service.ProposePlan(host, PlanInput{Edits: []TrackEdit{RenameEdit(original.Index, original.Name, planName)}})
	if err != nil || proposed.Plan == nil || proposed.Outcome != "pending" {
		t.Fatalf("plan proposal = %+v, %v", proposed, err)
	}
	beforeApply, err := service.ReadState(ctx, host)
	if err != nil || beforeApply.Tracks[len(beforeApply.Tracks)-1].Name != original.Name {
		t.Fatalf("proposal changed REAPER before apply: %+v, %v", beforeApply, err)
	}
	applied, err := service.ApplyPlan(ctx, host, PlanInput{PlanID: proposed.Plan.ID})
	if err != nil || applied.Outcome != "applied" {
		t.Fatalf("plan apply = %+v, %v", applied, err)
	}
	if _, err := service.RunAction(ctx, host, ActionInput{ActionID: "40029"}, false); err != nil {
		t.Fatalf("global undo after plan: %v", err)
	}

	cancelled, err := service.ProposePlan(host, PlanInput{Edits: []TrackEdit{MuteEdit(original.Index, original.Name, !original.Muted)}})
	if err != nil || cancelled.Plan == nil {
		t.Fatalf("cancel proposal = %+v, %v", cancelled, err)
	}
	if result, err := service.CancelPlan(host, PlanInput{PlanID: cancelled.Plan.ID}); err != nil || result.Outcome != "cancelled" {
		t.Fatalf("cancel = %+v, %v", result, err)
	}

	// The runner serializes concurrent drafts and reports real Lua failures.
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.RunDraft(ctx, DraftInput{Code: "local _ = reaper.CountTracks(0)"})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("serialized draft: %v", err)
		}
	}
	if _, err := service.RunDraft(ctx, DraftInput{Code: "error('ori parity deliberate error')"}); err == nil {
		t.Fatal("deliberate Lua failure was rendered as success")
	}

	// Agent proposals stay workspace-scoped, require a successful test before
	// global save, and discard without a library write.
	stamp := time.Now().UTC().Format("150405.000000000")
	filename := "ori-live-parity-" + strings.ReplaceAll(stamp, ".", "") + ".lua"
	proposal, err := service.ProposeScript(host, ProposalInput{
		Filename: filename, Name: "Ori live parity", Description: "Disposable live parity script",
		Code: "local _ = reaper.CountTracks(0)", NeedsConfirmation: true,
	})
	if err != nil || proposal.Proposal == nil {
		t.Fatalf("script proposal = %+v, %v", proposal, err)
	}
	if _, err := service.ReadProposal(HostContext{WorkspaceID: "foreign"}, ProposalInput{ProposalID: proposal.Proposal.ID}); err == nil {
		t.Fatal("foreign workspace read the proposal")
	}
	tested, err := service.TestProposal(ctx, host, ProposalInput{ProposalID: proposal.Proposal.ID})
	if err != nil || tested.Proposal == nil || !tested.Proposal.Tested {
		t.Fatalf("proposal test = %+v, %v", tested, err)
	}
	if _, err := service.SaveProposal(host, ProposalInput{ProposalID: proposal.Proposal.ID}); err != nil {
		t.Fatalf("proposal save: %v", err)
	}
	defer service.DeleteScript(ScriptIDInput{ID: filename})
	if _, err := service.ReadScript(ScriptIDInput{ID: filename}); err != nil {
		t.Fatalf("saved proposal is not global: %v", err)
	}

	discardName := "ori-live-discard-" + strings.ReplaceAll(stamp, ".", "") + ".lua"
	discard, err := service.ProposeScript(host, ProposalInput{Filename: discardName, Name: "Discard me", Code: "return 1"})
	if err != nil || discard.Proposal == nil {
		t.Fatalf("discard proposal = %+v, %v", discard, err)
	}
	if _, err := service.DiscardProposal(host, ProposalInput{ProposalID: discard.Proposal.ID}); err != nil {
		t.Fatalf("discard: %v", err)
	}
	if _, err := service.ReadScript(ScriptIDInput{ID: discardName}); err == nil {
		t.Fatal("discard wrote a global script")
	}

	final, err := service.ReadState(ctx, host)
	if err != nil || final.Tracks[len(final.Tracks)-1].Name != original.Name {
		t.Fatalf("live parity did not restore track identity: %+v, %v", final, err)
	}
	assertNoSensitiveLiveText(t, service.Station(ctx, host).Description)
}

func assertNoSensitiveLiveText(t *testing.T, value string) {
	t.Helper()
	lower := strings.ToLower(value)
	for _, forbidden := range []string{"2307", "2308", "/users/", ".ori-reaper", "inbox.lua", "last_receipt"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("live response leaked sensitive detail %q in %q", forbidden, value)
		}
	}
}

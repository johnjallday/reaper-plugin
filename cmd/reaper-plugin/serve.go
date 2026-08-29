package main

import (
	"context"
	"errors"

	"github.com/johnjallday/reaper-plugin/internal/reaper"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func runService() int {
	if err := reaper.ApplyServiceHomeOverride(); err != nil {
		return 1
	}
	roots := reaper.NewRunnerRootResolver()
	probes := reaper.NewPlatformProbeSet(roots)
	service := reaper.NewService(reaper.NewManagerFromEnv(), probes, roots)
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "reaper-plugin-service", Version: version}, nil)

	addTool(server, "service.version", "Return service/protocol health.", func(context.Context, reaper.Envelope[reaper.EmptyInput]) (reaper.ServiceInfo, error) {
		return service.Info(), nil
	})
	addTool(server, "status.read", "Return bounded REAPER station status.", func(ctx context.Context, input reaper.Envelope[reaper.EmptyInput]) (reaper.StationResult, error) {
		return service.Station(ctx, input.Context), nil
	})
	addTool(server, "runtime.prerequisites", "Check REAPER prerequisites.", func(ctx context.Context, input reaper.Envelope[reaper.EmptyInput]) (reaper.ReadyResult, error) {
		return service.Runtime().Prerequisites(ctx, input.Context), nil
	})
	addTool(server, "runtime.readiness", "Check durable REAPER readiness.", func(ctx context.Context, input reaper.Envelope[reaper.EmptyInput]) (reaper.ReadyResult, error) {
		return service.Runtime().Readiness(ctx, input.Context), nil
	})
	addTool(server, "runtime.live_status", "Check current REAPER availability.", func(ctx context.Context, input reaper.Envelope[reaper.EmptyInput]) (reaper.LiveStatusResult, error) {
		return service.Runtime().LiveStatus(ctx, input.Context), nil
	})
	addTool(server, "runtime.verify", "Run the bounded project-specific connection test.", func(ctx context.Context, input reaper.Envelope[reaper.EmptyInput]) (reaper.VerificationResult, error) {
		return service.Runtime().Verify(ctx, input.Context), nil
	})
	addTool(server, "runtime.repair", "Stage or recheck the trusted REAPER runner.", func(ctx context.Context, input reaper.Envelope[reaper.EmptyInput]) (reaper.RepairResult, error) {
		return service.Runtime().Repair(ctx, input.Context), nil
	})
	addTool(server, "state.read", "Read current REAPER project, transport, and tracks.", func(ctx context.Context, input reaper.Envelope[reaper.EmptyInput]) (reaper.State, error) {
		return service.ReadState(ctx, input.Context)
	})
	addTool(server, "actions.list", "List curated, registered, and custom actions.", func(context.Context, reaper.Envelope[reaper.EmptyInput]) (actionList, error) {
		actions, err := service.Actions()
		return actionList{Actions: actions}, err
	})
	addTool(server, "actions.run_safe", "Run a non-confirm tier action.", func(ctx context.Context, input reaper.Envelope[reaper.ActionInput]) (reaper.OperationResult, error) {
		return service.RunAction(ctx, input.Context, input.Input, false)
	})
	addTool(server, "actions.run_confirmed", "Run a host-confirmed destructive action.", func(ctx context.Context, input reaper.Envelope[reaper.ActionInput]) (reaper.OperationResult, error) {
		return service.RunAction(ctx, input.Context, input.Input, true)
	})
	addTool(server, "actions.run_raw_confirmed", "Run a host-confirmed raw command from the strict legacy grammar.", func(ctx context.Context, input reaper.Envelope[reaper.ActionInput]) (reaper.OperationResult, error) {
		return service.RunRawAction(ctx, input.Context, input.Input)
	})
	addTool(server, "scripts.list", "List shared custom scripts.", func(context.Context, reaper.Envelope[reaper.EmptyInput]) (scriptList, error) {
		scripts, err := service.Scripts()
		return scriptList{Scripts: scripts}, err
	})
	addTool(server, "scripts.read", "Read one shared custom script.", func(_ context.Context, input reaper.Envelope[reaper.ScriptIDInput]) (reaper.Script, error) {
		return service.ReadScript(input.Input)
	})
	addTool(server, "scripts.create", "Create one host-confirmed shared script.", func(_ context.Context, input reaper.Envelope[reaper.ScriptWriteInput]) (reaper.Script, error) {
		return service.CreateScript(input.Input)
	})
	addTool(server, "scripts.update", "Update one host-confirmed shared script.", func(_ context.Context, input reaper.Envelope[reaper.ScriptWriteInput]) (reaper.Script, error) {
		return service.UpdateScript(input.Input)
	})
	addTool(server, "scripts.delete", "Delete one host-confirmed shared script.", func(_ context.Context, input reaper.Envelope[reaper.ScriptIDInput]) (reaper.OperationResult, error) {
		return service.DeleteScript(input.Input)
	})
	addTool(server, "draft.validate", "Validate a proposed Lua draft without running it.", func(_ context.Context, input reaper.Envelope[reaper.DraftInput]) (reaper.DraftValidation, error) {
		return service.ValidateDraft(input.Input), nil
	})
	addTool(server, "draft.run", "Run a host-confirmed Lua draft through the registered runner.", func(ctx context.Context, input reaper.Envelope[reaper.DraftInput]) (reaper.OperationResult, error) {
		return service.RunDraft(ctx, input.Input)
	})
	addTool(server, "proposals.propose", "Propose a bounded workspace-scoped Lua script.", func(_ context.Context, input reaper.Envelope[reaper.ProposalInput]) (reaper.ProposalResult, error) {
		return service.ProposeScript(input.Context, input.Input)
	})
	addTool(server, "proposals.current", "Read the latest workspace-scoped script proposal.", func(_ context.Context, input reaper.Envelope[reaper.EmptyInput]) (reaper.ProposalResult, error) {
		return service.CurrentProposal(input.Context), nil
	})
	addTool(server, "proposals.read", "Read one workspace-scoped script proposal.", func(_ context.Context, input reaper.Envelope[reaper.ProposalInput]) (reaper.ProposalResult, error) {
		return service.ReadProposal(input.Context, input.Input)
	})
	addTool(server, "proposals.test", "Run one host-confirmed proposal test.", func(ctx context.Context, input reaper.Envelope[reaper.ProposalInput]) (reaper.ProposalResult, error) {
		return service.TestProposal(ctx, input.Context, input.Input)
	})
	addTool(server, "proposals.save", "Save one tested proposal to the global library.", func(_ context.Context, input reaper.Envelope[reaper.ProposalInput]) (reaper.ProposalResult, error) {
		return service.SaveProposal(input.Context, input.Input)
	})
	addTool(server, "proposals.discard", "Discard one proposal without a library write.", func(_ context.Context, input reaper.Envelope[reaper.ProposalInput]) (reaper.ProposalResult, error) {
		return service.DiscardProposal(input.Context, input.Input)
	})
	addTool(server, "tracks.edit", "Run one guarded reversible track edit.", func(ctx context.Context, input reaper.Envelope[reaper.TrackEditInput]) (reaper.TrackEditResult, error) {
		return service.RunTrackEdit(ctx, input.Context, input.Input)
	})
	addTool(server, "tracks.undo", "Undo the latest guarded track edit once.", func(ctx context.Context, input reaper.Envelope[reaper.EmptyInput]) (reaper.TrackEditResult, error) {
		return service.UndoTrackEdit(ctx, input.Context)
	})
	addTool(server, "plans.propose", "Propose a guarded workspace-scoped track plan.", func(_ context.Context, input reaper.Envelope[reaper.PlanInput]) (reaper.PlanResult, error) {
		return service.ProposePlan(input.Context, input.Input)
	})
	addTool(server, "plans.current", "Read the latest workspace-scoped guarded track plan.", func(_ context.Context, input reaper.Envelope[reaper.EmptyInput]) (reaper.PlanResult, error) {
		return service.CurrentPlan(input.Context), nil
	})
	addTool(server, "plans.read", "Read the current guarded track plan.", func(_ context.Context, input reaper.Envelope[reaper.PlanInput]) (reaper.PlanResult, error) {
		return service.ReadPlan(input.Context, input.Input)
	})
	addTool(server, "plans.apply", "Apply one host-confirmed guarded track plan.", func(ctx context.Context, input reaper.Envelope[reaper.PlanInput]) (reaper.PlanResult, error) {
		return service.ApplyPlan(ctx, input.Context, input.Input)
	})
	addTool(server, "plans.cancel", "Cancel one workspace-scoped guarded track plan.", func(_ context.Context, input reaper.Envelope[reaper.PlanInput]) (reaper.PlanResult, error) {
		return service.CancelPlan(input.Context, input.Input)
	})
	addTool(server, "results.read", "Read the last bounded service operation result.", func(context.Context, reaper.Envelope[reaper.EmptyInput]) (reaper.OperationResult, error) {
		return service.LastResult(), nil
	})
	addTool(server, "tidy.survey", "Run the canonical read-only inspector and persist a cosmetic-only Project Tidy proposal.", func(ctx context.Context, input reaper.Envelope[reaper.EmptyInput]) (reaper.TidySurveyResult, error) {
		return service.RunTidySurvey(ctx, input.Context)
	})
	addTool(server, "tidy.station", "Read Project Tidy station status for the host project.", func(ctx context.Context, input reaper.Envelope[reaper.EmptyInput]) (reaper.StationResult, error) {
		return service.TidyStation(ctx, input.Context), nil
	})
	addTool(server, "tidy.status", "Read bounded Project Tidy proposal status for the host project.", func(_ context.Context, input reaper.Envelope[reaper.EmptyInput]) (reaper.TidySurfaceStatus, error) {
		return service.TidyStatus(input.Context)
	})
	addTool(server, "tidy.proposal.read", "Read the latest or selected human Project Tidy proposal projection.", func(_ context.Context, input reaper.Envelope[reaper.TidyProposalReadInput]) (reaper.TidyProposalView, error) {
		return service.ReadTidyProposal(input.Context, input.Input)
	})
	addTool(server, "tidy.proposal.dismiss", "Dismiss one open Project Tidy proposal without changing REAPER.", func(_ context.Context, input reaper.Envelope[reaper.TidyProposalActionInput]) (reaper.OperationResult, error) {
		return service.DismissTidyProposal(input.Context, input.Input)
	})

	if err := server.Run(context.Background(), &sdkmcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		return 1
	}
	return 0
}

type actionList struct {
	Actions []reaper.Action `json:"actions"`
}

type scriptList struct {
	Scripts []reaper.Script `json:"scripts"`
}

func addTool[Input, Output any](server *sdkmcp.Server, name, description string, call func(context.Context, reaper.Envelope[Input]) (Output, error)) {
	sdkmcp.AddTool(server, &sdkmcp.Tool{Name: name, Description: description}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input reaper.Envelope[Input]) (*sdkmcp.CallToolResult, Output, error) {
		output, err := call(ctx, input)
		return &sdkmcp.CallToolResult{}, output, err
	})
}

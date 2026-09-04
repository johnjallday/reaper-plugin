package reaper

import (
	"context"
	"path/filepath"
	"strings"
	"time"
)

// HostContext is injected by Ori after authorization. Service inputs cannot
// select a workspace, project path, runner root, endpoint, grant, or scope.
type HostContext struct {
	WorkspaceID    string   `json:"workspace_id"`
	WorkspaceRoot  string   `json:"workspace_root,omitempty"`
	ProjectEntry   string   `json:"project_entry,omitempty"`
	PluginDataRoot string   `json:"plugin_data_root,omitempty"`
	Scopes         []string `json:"scopes,omitempty"`
}

type RuntimeRepairReview struct {
	Destination        string `json:"destination"`
	ManualRegistration string `json:"manual_registration"`
}

type ReadyResult struct {
	Ready        bool                 `json:"ready"`
	Summary      string               `json:"summary"`
	RepairReview *RuntimeRepairReview `json:"repair_review,omitempty"`
}

type LiveStatusResult struct {
	Available bool   `json:"available"`
	Summary   string `json:"summary"`
}

type VerificationResult struct {
	Verified   bool   `json:"verified"`
	Summary    string `json:"summary"`
	ReasonCode string `json:"reason_code,omitempty"`
}

type RepairResult struct {
	Repaired bool   `json:"repaired"`
	Summary  string `json:"summary"`
}

// RuntimeProvider owns REAPER domain probes only. Ori remains responsible for
// mode selection, plugin attachment, compatible-agent selection, grants,
// symbolic scope resolution, confirmation, and final execution confinement.
type RuntimeProvider struct {
	manager *Manager
	probes  ProbeSet
}

func NewRuntimeProvider(manager *Manager, probes ProbeSet) *RuntimeProvider {
	return &RuntimeProvider{manager: manager, probes: probes}
}

func (p *RuntimeProvider) Prerequisites(ctx context.Context, _ HostContext) ReadyResult {
	if p == nil || p.probes.Application == nil || p.probes.WebRemote == nil || p.probes.Runner == nil {
		return ReadyResult{Summary: "REAPER runtime checks are unavailable on this platform."}
	}
	application := p.probes.Application.DetectApplication(ctx)
	if application.State == ProbeUnsupported {
		return ReadyResult{Summary: "Guided REAPER control is unsupported on this platform. File-only work remains available."}
	}
	if application.State != ProbeReady {
		return ReadyResult{Summary: "Install REAPER in a supported application location, then check again. Ori does not install or open it automatically."}
	}
	web := p.probes.WebRemote.DetectWebRemote(ctx)
	if web.State != ProbeReady {
		return ReadyResult{Summary: "In REAPER, open Preferences, choose Control/OSC/web, add the Web browser interface, enable it, then check again. Ori does not change REAPER preferences."}
	}
	runner := p.probes.Runner.DetectRunner(ctx)
	if runner.State != ProbeReady {
		result := ReadyResult{Summary: "The Ori REAPER runner is not registered yet."}
		if destination := p.manager.RunnerScriptPath(); destination != "" {
			result.RepairReview = &RuntimeRepairReview{
				Destination:        destination,
				ManualRegistration: "After staging, open REAPER's Action List, load the staged script, and run it once. Staging alone does not prove registration.",
			}
		}
		return result
	}
	return ReadyResult{Ready: true, Summary: "REAPER, Web Remote, and the registered runner are available."}
}

func (p *RuntimeProvider) Readiness(ctx context.Context, host HostContext) ReadyResult {
	result := p.Prerequisites(ctx, host)
	if !result.Ready {
		return result
	}
	if !validHostProject(host.ProjectEntry) {
		return ReadyResult{Summary: "The workspace's authoritative REAPER project is unavailable."}
	}
	return ReadyResult{Ready: true, Summary: "REAPER live-control prerequisites are configured for this workspace project."}
}

func (p *RuntimeProvider) LiveStatus(ctx context.Context, host HostContext) LiveStatusResult {
	ready := p.Readiness(ctx, host)
	if !ready.Ready || p.probes.Transport == nil || p.probes.WebRemote == nil {
		return LiveStatusResult{Summary: ready.Summary}
	}
	web := p.probes.WebRemote.DetectWebRemote(ctx)
	transport := p.probes.Transport.CheckTransport(ctx, web)
	if transport.State != TransportAvailable {
		return LiveStatusResult{Summary: "REAPER is offline or its Web Remote interface is unavailable."}
	}
	return LiveStatusResult{Available: true, Summary: "REAPER Web Remote is responding."}
}

func (p *RuntimeProvider) Verify(ctx context.Context, host HostContext) VerificationResult {
	if p == nil || p.probes.WebRemote == nil || p.probes.Runner == nil || p.probes.Transport == nil || p.probes.Verifier == nil {
		return VerificationResult{Summary: "The REAPER connection could not be verified.", ReasonCode: "verification_unavailable"}
	}
	if !validHostProject(host.ProjectEntry) {
		return VerificationResult{Summary: "The workspace's authoritative REAPER project is unavailable.", ReasonCode: "unsafe_project"}
	}
	if ready := p.Prerequisites(ctx, host); !ready.Ready {
		return VerificationResult{Summary: ready.Summary, ReasonCode: "prerequisite_missing"}
	}
	web := p.probes.WebRemote.DetectWebRemote(ctx)
	transport := p.probes.Transport.CheckTransport(ctx, web)
	if transport.State != TransportAvailable {
		return VerificationResult{Summary: "REAPER is offline or its Web Remote interface is unavailable.", ReasonCode: "offline"}
	}
	if transport.Port > 0 {
		web.Port = transport.Port
	}
	result := p.probes.Verifier.VerifyProject(ctx, VerificationTarget{
		ExpectedProject: host.ProjectEntry, WebRemote: web, Runner: p.probes.Runner.DetectRunner(ctx), Timeout: 6 * time.Second,
	})
	if result.State != VerificationSucceeded {
		return VerificationResult{Summary: verificationSummary(result.State), ReasonCode: verificationReasonCode(result.State)}
	}
	return VerificationResult{Verified: true, Summary: "The trusted REAPER connection test matched this workspace project."}
}

func (p *RuntimeProvider) Repair(ctx context.Context, host HostContext) RepairResult {
	if ready := p.Readiness(ctx, host); ready.Ready {
		return RepairResult{Repaired: true, Summary: "REAPER live control is already configured."}
	}
	if p == nil || p.manager == nil || p.probes.Runner == nil {
		return RepairResult{Summary: "REAPER setup must be completed manually."}
	}
	if p.probes.Runner.DetectRunner(ctx).State == ProbeReady {
		return RepairResult{Summary: "Review REAPER and Web Remote setup, then check again."}
	}
	if _, err := p.manager.InstallRunner(); err != nil {
		return RepairResult{Summary: "The REAPER runner could not be staged."}
	}
	return RepairResult{Summary: "The runner was staged. Load and run it once from REAPER's Action List, then check again."}
}

func validHostProject(path string) bool {
	path = strings.TrimSpace(path)
	return filepath.IsAbs(path) && strings.HasSuffix(strings.ToLower(path), ".rpp")
}

func verificationReasonCode(state string) string {
	switch state {
	case VerificationWrongProject:
		return "wrong_project"
	case VerificationProjectMissing:
		return "no_project"
	case VerificationTimedOut:
		return "timeout"
	case VerificationPermissionDenied:
		return "unsafe_exchange"
	case VerificationRunnerFailed:
		return "runner_failed"
	default:
		return "invalid_response"
	}
}

func verificationSummary(state string) string {
	switch state {
	case VerificationWrongProject:
		return "REAPER has a different project open."
	case VerificationProjectMissing:
		return "REAPER did not report an open project."
	case VerificationTimedOut:
		return "The REAPER connection test timed out."
	case VerificationPermissionDenied:
		return "The REAPER runner exchange is not safely writable."
	case VerificationRunnerFailed:
		return "The REAPER runner could not complete the connection test."
	default:
		return "The REAPER connection test returned an invalid result."
	}
}

package reaper

import (
	"context"
	"testing"
)

type runtimeProbe struct {
	application  ApplicationObservation
	web          WebRemoteObservation
	runner       RunnerObservation
	transport    LiveTransportObservation
	verification VerificationObservation
	target       VerificationTarget
}

func (p *runtimeProbe) DetectApplication(context.Context) ApplicationObservation {
	return p.application
}
func (p *runtimeProbe) DetectWebRemote(context.Context) WebRemoteObservation { return p.web }
func (p *runtimeProbe) DetectRunner(context.Context) RunnerObservation       { return p.runner }
func (p *runtimeProbe) CheckTransport(context.Context, WebRemoteObservation) LiveTransportObservation {
	return p.transport
}
func (p *runtimeProbe) VerifyProject(_ context.Context, target VerificationTarget) VerificationObservation {
	p.target = target
	return p.verification
}

func readyRuntimeProbe() *runtimeProbe {
	return &runtimeProbe{
		application:  ApplicationObservation{State: ProbeReady},
		web:          WebRemoteObservation{State: ProbeReady, Port: 2307},
		runner:       RunnerObservation{State: ProbeReady, Root: "/trusted/runner", CommandID: "_RSabc123"},
		transport:    LiveTransportObservation{State: TransportAvailable, Port: 2308},
		verification: VerificationObservation{State: VerificationSucceeded},
	}
}

func runtimeProviderWith(probe *runtimeProbe) *RuntimeProvider {
	return NewRuntimeProvider(&Manager{}, ProbeSet{Application: probe, WebRemote: probe, Runner: probe, Transport: probe, Verifier: probe})
}

func TestRuntimeProviderChecksInjectedDomainFactsAndHostProject(t *testing.T) {
	probe := readyRuntimeProbe()
	provider := runtimeProviderWith(probe)
	host := HostContext{WorkspaceID: "workspace-a", ProjectEntry: "/trusted/project/Song.rpp"}
	if result := provider.Prerequisites(context.Background(), host); !result.Ready {
		t.Fatalf("prerequisites = %+v", result)
	}
	if result := provider.Readiness(context.Background(), host); !result.Ready {
		t.Fatalf("readiness = %+v", result)
	}
	if result := provider.LiveStatus(context.Background(), host); !result.Available {
		t.Fatalf("live = %+v", result)
	}
	if result := provider.Verify(context.Background(), host); !result.Verified {
		t.Fatalf("verify = %+v", result)
	}
	if probe.target.ExpectedProject != host.ProjectEntry || probe.target.WebRemote.Port != 2308 {
		t.Fatalf("verification target = %+v", probe.target)
	}
}

func TestRuntimeProviderFailsClosedWithoutPlatformProjectOrRunner(t *testing.T) {
	probe := readyRuntimeProbe()
	probe.application.State = ProbeUnsupported
	provider := runtimeProviderWith(probe)
	if result := provider.Prerequisites(context.Background(), HostContext{}); result.Ready {
		t.Fatalf("unsupported prerequisites = %+v", result)
	}
	probe.application.State = ProbeReady
	if result := provider.Readiness(context.Background(), HostContext{ProjectEntry: "../caller.rpp"}); result.Ready {
		t.Fatalf("caller path was accepted = %+v", result)
	}
	probe.runner.State = ProbeMissing
	if result := provider.Verify(context.Background(), HostContext{ProjectEntry: "/trusted/Song.rpp"}); result.Verified {
		t.Fatalf("missing runner verification = %+v", result)
	}
}

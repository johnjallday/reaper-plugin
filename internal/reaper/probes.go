package reaper

import (
	"context"
	"time"
)

// Probe DTOs are plugin-owned service-boundary types. They deliberately mirror
// observable REAPER setup facts without importing ori-agent/internal packages.
type ProbeState string

const (
	ProbeReady       ProbeState = "ready"
	ProbeMissing     ProbeState = "missing"
	ProbeInvalid     ProbeState = "invalid"
	ProbeUnknown     ProbeState = "unknown"
	ProbeUnsupported ProbeState = "unsupported"
)

type TransportState string

const (
	TransportAvailable   TransportState = "available"
	TransportOffline     TransportState = "offline"
	TransportUnavailable TransportState = "unavailable"
	TransportMalformed   TransportState = "malformed"
	TransportCheckFailed TransportState = "check_failed"
	TransportError       TransportState = TransportCheckFailed
)

type ApplicationObservation struct {
	State ProbeState `json:"state"`
}

type WebRemoteObservation struct {
	State ProbeState `json:"state"`
	Port  int        `json:"port,omitempty"`
	Ports []int      `json:"ports,omitempty"`
}

type LiveTransportObservation struct {
	State TransportState `json:"state"`
	Port  int            `json:"port,omitempty"`
}

type RunnerObservation struct {
	State     ProbeState `json:"state"`
	Root      string     `json:"root,omitempty"`
	CommandID string     `json:"command_id,omitempty"`
}

const (
	VerificationSucceeded        = "succeeded"
	VerificationWrongProject     = "wrong_project"
	VerificationProjectMissing   = "project_missing"
	VerificationTimedOut         = "timeout"
	VerificationRunnerFailed     = "runner_failed"
	VerificationPermissionDenied = "permission_denied"
	VerificationMalformed        = "malformed"
	VerificationCheckFailed      = "check_failed"
)

type VerificationTarget struct {
	ExpectedProject string
	WebRemote       WebRemoteObservation
	Runner          RunnerObservation
	Timeout         time.Duration
}

type VerificationObservation struct {
	State string `json:"state"`
}

type ApplicationProbe interface {
	DetectApplication(context.Context) ApplicationObservation
}

type WebRemoteProbe interface {
	DetectWebRemote(context.Context) WebRemoteObservation
}

type LiveTransportProbe interface {
	CheckTransport(context.Context, WebRemoteObservation) LiveTransportObservation
}

type RunnerProbe interface {
	DetectRunner(context.Context) RunnerObservation
}

type ProjectVerifier interface {
	VerifyProject(context.Context, VerificationTarget) VerificationObservation
}

type platformProber interface {
	ApplicationProbe
	WebRemoteProbe
	RunnerProbe
	LiveTransportProbe
	ProjectVerifier
}

type ProbeSet struct {
	Application ApplicationProbe
	WebRemote   WebRemoteProbe
	Transport   LiveTransportProbe
	Runner      RunnerProbe
	Verifier    ProjectVerifier
}

func NewPlatformProbeSet(roots RunnerRootResolver) ProbeSet {
	probe := newPlatformProbe(roots)
	return ProbeSet{Application: probe, WebRemote: probe, Transport: probe, Runner: probe, Verifier: probe}
}

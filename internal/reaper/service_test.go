package reaper

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func actionServiceFixture(t *testing.T) (*Service, *int, HostContext) {
	t.Helper()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/_/1007", "/_/1013":
			calls++
		case "/_/TRANSPORT":
			_, _ = w.Write([]byte("TRANSPORT\t0\t0\t0\t1.1.00\t1.1.00\n"))
		case "/_/TRACK":
			_, _ = w.Write([]byte("TRACK\t0\tMASTER\t1536\t1\t0\t-1500\t-1500\t1\t0\t0\t0\t1\t0\n"))
		case "/_/BEATPOS":
			_, _ = w.Write([]byte("BEATPOS\t0\t0\t0\t0\t0\t4\t4\n"))
		default:
			http.NotFound(w, request)
		}
	}))
	t.Cleanup(server.Close)
	port, _ := strconv.Atoi(strings.TrimPrefix(server.URL, "http://127.0.0.1:"))
	probe := clientProbe{
		web:       WebRemoteObservation{State: ProbeReady, Port: port},
		transport: LiveTransportObservation{State: TransportAvailable, Port: port},
	}
	client := NewClient(ProbeSet{WebRemote: probe, Transport: probe})
	client.http = server.Client()
	project := filepath.Join(t.TempDir(), "Service.rpp")
	if err := os.WriteFile(project, []byte("<REAPER_PROJECT\nTEMPO 120 4 4\n>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := &Service{client: client, catalog: NewCatalogWithKeyboardConfig("")}
	return service, &calls, HostContext{WorkspaceID: "workspace-a", ProjectEntry: project}
}

func TestServiceActionTiersConvergeOnOneDomainPath(t *testing.T) {
	service, calls, host := actionServiceFixture(t)
	if _, err := service.RunAction(context.Background(), host, ActionInput{ActionID: "1007"}, false); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 {
		t.Fatalf("safe action calls = %d", *calls)
	}
	if _, err := service.RunAction(context.Background(), host, ActionInput{ActionID: "1013"}, false); err == nil || *calls != 1 {
		t.Fatalf("unconfirmed destructive action = %v calls=%d", err, *calls)
	}
	if _, err := service.RunAction(context.Background(), host, ActionInput{ActionID: "1013"}, true); err != nil || *calls != 2 {
		t.Fatalf("confirmed action = %v calls=%d", err, *calls)
	}
}

func TestServiceRawCommandRejectsBrowserStylePayloadAndSanitizesFailure(t *testing.T) {
	service, calls, host := actionServiceFixture(t)
	for _, id := range []string{"1007/TRANSPORT", "http://127.0.0.1:2307", "../1007", "_RS"} {
		if _, err := service.RunRawAction(context.Background(), host, ActionInput{ActionID: id}); err == nil {
			t.Fatalf("raw id %q was accepted", id)
		}
	}
	if *calls != 0 {
		t.Fatalf("rejected raw commands reached REAPER: %d", *calls)
	}
	service.client.http = &http.Client{Transport: failingRoundTripper{}}
	_, err := service.RunRawAction(context.Background(), host, ActionInput{ActionID: "1007"})
	if err == nil || strings.Contains(err.Error(), host.ProjectEntry) || strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("raw failure leaked internals: %v", err)
	}
}

func TestServiceScriptProposalsAreWorkspaceScopedAndCannotSaveUntested(t *testing.T) {
	library := NewLibraryAt(filepath.Join(t.TempDir(), "scripts"))
	service := &Service{proposals: make(map[string]ScriptProposal), library: library}
	hostA := HostContext{WorkspaceID: "workspace-a"}
	hostB := HostContext{WorkspaceID: "workspace-b"}
	proposed, err := service.ProposeScript(hostA, ProposalInput{
		Filename: "agent-proposal.lua", Name: "Agent proposal", Description: "Harmless test",
		Code: "return 1", NeedsConfirmation: true,
	})
	if err != nil || proposed.Proposal == nil {
		t.Fatalf("proposal = %+v, %v", proposed, err)
	}
	id := proposed.Proposal.ID
	if _, err := service.ReadProposal(hostB, ProposalInput{ProposalID: id}); err == nil {
		t.Fatal("another workspace read the script proposal")
	}
	if _, err := service.SaveProposal(hostA, ProposalInput{ProposalID: id}); err == nil {
		t.Fatal("untested proposal was saved globally")
	}
	if _, err := library.Read("agent-proposal.lua"); !errors.Is(err, ErrScriptNotFound) {
		t.Fatalf("untested proposal wrote a file: %v", err)
	}
	if result, err := service.DiscardProposal(hostA, ProposalInput{ProposalID: id}); err != nil || result.Outcome != "discarded" {
		t.Fatalf("discard = %+v, %v", result, err)
	}
}

func TestServicePlansAreOpaqueWorkspaceScopedAndAgentCannotApply(t *testing.T) {
	service := &Service{plans: make(map[string]PendingPlan)}
	hostA := HostContext{WorkspaceID: "workspace-a"}
	hostB := HostContext{WorkspaceID: "workspace-b"}
	proposed, err := service.ProposePlan(hostA, PlanInput{Edits: []TrackEdit{RenameEdit(1, "Old", "New")}})
	if err != nil || proposed.Plan == nil || proposed.Plan.ID == "" {
		t.Fatalf("proposed = %+v, %v", proposed, err)
	}
	if _, err := service.ReadPlan(hostB, PlanInput{PlanID: proposed.Plan.ID}); err == nil {
		t.Fatal("another workspace read the opaque plan")
	}
	cancelled, err := service.CancelPlan(hostA, PlanInput{PlanID: proposed.Plan.ID})
	if err != nil || cancelled.Outcome != "cancelled" {
		t.Fatalf("cancelled = %+v, %v", cancelled, err)
	}
	if _, err := service.ReadPlan(hostA, PlanInput{PlanID: proposed.Plan.ID}); err == nil {
		t.Fatal("cancelled plan remained available")
	}

	data, err := os.ReadFile(filepath.Join("..", "..", ".ori-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest contributionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, operation := range manifest.Capabilities[0].AgentOperations {
		if operation == "plans.apply" || operation == "plans.cancel" {
			t.Fatalf("agent can bypass host plan review through %q", operation)
		}
	}
}

type failingRoundTripper struct{}

func (failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, os.ErrPermission
}

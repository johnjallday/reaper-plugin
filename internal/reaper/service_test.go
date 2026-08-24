package reaper

import (
	"context"
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

type failingRoundTripper struct{}

func (failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, os.ErrPermission
}

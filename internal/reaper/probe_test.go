package reaper

import (
	"strings"
	"testing"
)

func TestParseWebRemoteConfig(t *testing.T) {
	tests := []struct {
		name  string
		input string
		state ProbeState
		port  int
	}{
		{name: "real REAPER HTTP encoding", input: "[reaper]\ncsurf_0=HTTP 0 2307 '' 'index.html' 0 ''\n", state: ProbeReady, port: 2307},
		{name: "alternate HTTP mode", input: "csurf_0=HTTP 1 2308 '' 'index.html' 0 ''\n", state: ProbeReady, port: 2308},
		{name: "WEBR enabled", input: "csurf_2=WEBR 1 0 3210\n", state: ProbeReady, port: 3210},
		{name: "WEBR disabled", input: "csurf_2=WEBR 0 0 3210\n", state: ProbeMissing},
		{name: "default guess is not configuration", input: "lastproject=/music/song.rpp\n", state: ProbeMissing},
		{name: "bad port", input: "csurf_0=HTTP 1 70000\n", state: ProbeInvalid},
		{name: "bad enabled flag", input: "csurf_0=HTTP maybe 2307\n", state: ProbeInvalid},
		{name: "truncated", input: "csurf_0=HTTP 1\n", state: ProbeInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := parseWebRemoteConfig([]byte(test.input))
			if got.State != test.state || got.Port != test.port {
				t.Fatalf("got %+v, want state=%s port=%d", got, test.state, test.port)
			}
		})
	}
	multiple := parseWebRemoteConfig([]byte("csurf_0=HTTP 0 2307\ncsurf_1=HTTP 0 2308\ncsurf_2=HTTP 0 2308\n"))
	if multiple.State != ProbeReady || multiple.Port != 2307 || len(multiple.Ports) != 2 {
		t.Fatalf("multiple configured interfaces = %+v", multiple)
	}
	if oversized := parseWebRemoteConfig([]byte(strings.Repeat("x", maxREAPERConfigBytes+1))); oversized.State != ProbeUnknown {
		t.Fatalf("oversized state = %s", oversized.State)
	}
}

func TestRunnerCommandIDValidation(t *testing.T) {
	for _, valid := range []string{"_RSabc123", "_CUSTOM-42", "12345"} {
		if !validRunnerCommandID(valid) {
			t.Errorf("%q should be valid", valid)
		}
	}
	for _, invalid := range []string{"", "0", "_", "../runner", "id with spaces", strings.Repeat("A", 100)} {
		if validRunnerCommandID(invalid) {
			t.Errorf("%q should be invalid", invalid)
		}
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/johnjallday/reaper-plugin/internal/reaper"
)

func TestReleaseBuildPinsTheModulePatchToolchain(t *testing.T) {
	module, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^go ([0-9]+\.[0-9]+\.[0-9]+)$`).FindStringSubmatch(string(module))
	if len(match) != 2 {
		t.Fatal("release module must pin an exact Go patch toolchain")
	}
	build, err := os.ReadFile("../../scripts/build-local-artifact.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(build), "release_toolchain=go"+match[1]+"\n") {
		t.Fatal("artifact build must use the same patched toolchain as CI/tests")
	}
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), "pins Go "+match[1]) {
		t.Fatal("documented build toolchain differs from the actual release")
	}
}

func TestCurrentReleaseMetadataMatchesServiceAndDocs(t *testing.T) {
	data, err := os.ReadFile("../../.ori-plugin/plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Name     string `json:"name"`
		Version  string `json:"version"`
		Services []struct {
			Artifacts []struct {
				OS     string `json:"os"`
				Arch   string `json:"arch"`
				SHA256 string `json:"sha256"`
				Size   int64  `json:"size"`
				Source struct {
					Kind string `json:"kind"`
					URL  string `json:"url"`
				} `json:"source"`
			} `json:"artifacts"`
		} `json:"services"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	info := (*reaper.Service)(nil).Info()
	if manifest.Version != version || info.Version != version || manifest.Name != info.Name {
		t.Fatalf("release identity mismatch: manifest=%s/%s CLI=%s service=%+v", manifest.Name, manifest.Version, version, info)
	}
	data, err = os.ReadFile("../../.claude-plugin/plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	var portable struct{ Name, Version string }
	if err := json.Unmarshal(data, &portable); err != nil {
		t.Fatal(err)
	}
	if portable.Name != manifest.Name || portable.Version != version {
		t.Fatalf("portable identity mismatch: %+v", portable)
	}
	if len(manifest.Services) != 1 || len(manifest.Services[0].Artifacts) != 1 {
		t.Fatal("the current release must declare exactly one reviewed service artifact")
	}
	artifact := manifest.Services[0].Artifacts[0]
	wantURL := fmt.Sprintf("https://github.com/johnjallday/reaper-plugin/releases/download/v%s/reaper-plugin_v%s_darwin_arm64", version, version)
	if artifact.OS != "darwin" || artifact.Arch != "arm64" || artifact.Source.Kind != "https" || artifact.Source.URL != wantURL ||
		artifact.Size <= 0 || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(artifact.SHA256) {
		t.Fatalf("invalid release artifact identity: %+v", artifact)
	}
	for _, name := range []string{"README.md", "RELEASE_NOTES.md"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile("../../" + name) // #nosec G304 -- fixed repository documentation files
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			if name == "RELEASE_NOTES.md" {
				_, current, found := strings.Cut(text, "\n## "+version+" ")
				if !found {
					t.Fatalf("missing current %s release notes", version)
				}
				text, _, _ = strings.Cut(current, "\n## ")
			}
			plain := strings.NewReplacer(",", "", "`", "").Replace(text)
			if !strings.Contains(plain, fmt.Sprintf("%d bytes", artifact.Size)) || !strings.Contains(plain, artifact.SHA256) {
				t.Fatalf("%s must describe current manifest bytes (%d, sha256=%s), not an older release", name, artifact.Size, artifact.SHA256)
			}
		})
	}
}

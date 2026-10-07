package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// fixtureHome is a home directory that holds a REAPER installation in the
// user's own Applications folder and a per-user resource folder with two
// project templates, one track template and a file that is not a template.
// The service is pointed at it, so the test never reads the developer's own
// REAPER settings.
func fixtureHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	contents := filepath.Join(home, "Applications", "REAPER.app", "Contents")
	resource := filepath.Join(home, "Library", "Application Support", "REAPER")
	for _, dir := range []string{contents, filepath.Join(resource, "ProjectTemplates"), filepath.Join(resource, "TrackTemplates")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	info := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>CFBundleShortVersionString</key><string>7.28.0_fixture</string></dict></plist>
`
	files := map[string]string{
		filepath.Join(contents, "Info.plist"):                                info,
		filepath.Join(resource, "ProjectTemplates", "Band Session.RPP"):      "<REAPER_PROJECT\n>\n",
		filepath.Join(resource, "ProjectTemplates", "Vocal & Comp.RPP"):      "<REAPER_PROJECT\n>\n",
		filepath.Join(resource, "ProjectTemplates", "notes.txt"):             "not a template",
		filepath.Join(resource, "TrackTemplates", "Drum Bus.RTrackTemplate"): "<TRACK\n>\n",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func TestPrivateServiceConformsToMCPStdioAndDeclaredHealthOperation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "run", ".", "serve") // #nosec G204 -- fixed test command/arguments
	command.Env = append(os.Environ(), "REAPER_PLUGIN_HOME="+fixtureHome(t))
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "reaper-plugin-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "service.version",
		Arguments: map[string]any{
			"protocol_version": 1,
			"operation_id":     "service.version",
			"context":          map[string]any{"workspace_id": "fixture"},
			"input":            map[string]any{},
		},
	})
	if err != nil || result.IsError {
		t.Fatalf("service.version = %+v, %v", result, err)
	}
	content, ok := result.StructuredContent.(map[string]any)
	if !ok || content["name"] != "reaper-plugin" || content["protocol_version"] != float64(1) || content["healthy"] != true {
		t.Fatalf("structured health = %#v", result.StructuredContent)
	}

	// The profile read is called by the host itself with an empty context: no
	// workspace, project or scope.
	read := func(input map[string]any) (*sdkmcp.CallToolResult, error) {
		return session.CallTool(ctx, &sdkmcp.CallToolParams{
			Name: "profile.read",
			Arguments: map[string]any{
				"protocol_version": 1,
				"operation_id":     "profile.read",
				"context": map[string]any{
					"workspace_id": "", "workspace_root": "", "project_entry": "", "plugin_data_root": "", "scopes": []string{},
				},
				"input": input,
			},
		})
	}

	// Asked for installation and version only, it names no template.
	result, err = read(map[string]any{"include_templates": false})
	if err != nil || result.IsError {
		t.Fatalf("profile.read = %+v, %v", result, err)
	}
	facts, ok := result.StructuredContent.(map[string]any)
	if !ok || facts["app"] != "REAPER" || facts["installed"] != true {
		t.Fatalf("structured profile facts = %#v", result.StructuredContent)
	}
	if version, _ := facts["version"].(string); version == "" {
		t.Fatalf("no version in %#v", facts)
	}
	if _, listed := facts["templates"]; listed || facts["truncated"] != false || facts["templates_available"] == nil {
		t.Fatalf("a version-only read named templates: %#v", facts)
	}

	// Asked for templates, the answer crosses the service's own declared
	// output shape with a populated list.
	result, err = read(map[string]any{"include_templates": true})
	if err != nil || result.IsError {
		t.Fatalf("profile.read with templates = %+v, %v", result, err)
	}
	facts, ok = result.StructuredContent.(map[string]any)
	if !ok || facts["app"] != "REAPER" {
		t.Fatalf("structured profile facts = %#v", result.StructuredContent)
	}
	// A computer whose system REAPER is a portable installation keeps its
	// settings beside the application, not in the fixture home; the names
	// below are then that installation's, and only the shape is checked.
	if _, err := os.Lstat("/Applications/reaper.ini"); err != nil {
		listed, _ := facts["templates"].([]any)
		got := make([]string, 0, len(listed))
		for _, entry := range listed {
			template, _ := entry.(map[string]any)
			kind, _ := template["kind"].(string)
			name, _ := template["name"].(string)
			file, _ := template["file"].(string)
			if _, hasTime := template["modified_at"].(string); !hasTime || file == "" {
				t.Fatalf("template = %#v", template)
			}
			got = append(got, kind+":"+name)
		}
		want := []string{"project:Band Session", "project:Vocal & Comp", "track:Drum Bus"}
		if len(got) != len(want) {
			t.Fatalf("templates = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("templates = %v, want %v", got, want)
			}
		}
		if facts["truncated"] != false || facts["templates_available"] != true {
			t.Fatalf("facts = %#v", facts)
		}
	}

	// The one input is required: a call without it is refused, not defaulted.
	if result, err = read(map[string]any{}); err == nil && !result.IsError {
		t.Fatalf("profile.read without include_templates was accepted: %#v", result.StructuredContent)
	}
	if result, err = read(map[string]any{"include_templates": true, "folder": "/Users/me"}); err == nil && !result.IsError {
		t.Fatalf("profile.read with an extra input was accepted: %#v", result.StructuredContent)
	}
}

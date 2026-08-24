package reaper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type contributionManifest struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	Capabilities []struct {
		ID              string   `json:"id"`
		AgentOperations []string `json:"agent_operations"`
	} `json:"capabilities"`
}

func TestContributionDeclaresGrantGatedAgentOperationsWithoutPortableMCP(t *testing.T) {
	root := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(root, ".ori-plugin", "plugin.json")) // #nosec G304 -- fixed repository manifest
	if err != nil {
		t.Fatal(err)
	}
	var manifest contributionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Name != "reaper-plugin" || manifest.Version != "0.3.0" || len(manifest.Capabilities) != 1 {
		t.Fatalf("manifest = %+v", manifest)
	}
	operations := map[string]bool{}
	for _, id := range manifest.Capabilities[0].AgentOperations {
		operations[id] = true
	}
	for _, required := range []string{"state.read", "actions.list", "actions.run_safe", "actions.run_confirmed", "scripts.list", "draft.validate", "draft.run"} {
		if !operations[required] {
			t.Errorf("agent operation %q is not declared", required)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatalf("private service must not be exposed as portable native MCP: %v", err)
	}
}

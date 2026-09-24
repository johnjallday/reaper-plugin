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
	Services []struct {
		Operations []struct {
			ID     string   `json:"id"`
			Policy string   `json:"policy"`
			Scopes []string `json:"scopes"`
		} `json:"operations"`
	} `json:"services"`
}

func TestContributionDeclaresBoundedTidyReadOperations(t *testing.T) {
	root := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(root, ".ori-plugin", "plugin.json")) // #nosec G304 -- fixed repository manifest
	if err != nil {
		t.Fatal(err)
	}
	var manifest contributionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Services) != 1 {
		t.Fatalf("services = %+v", manifest.Services)
	}
	operations := make(map[string]struct {
		policy string
		scopes int
	})
	for _, operation := range manifest.Services[0].Operations {
		operations[operation.ID] = struct {
			policy string
			scopes int
		}{operation.Policy, len(operation.Scopes)}
	}
	for _, id := range []string{"tidy.status", "tidy.proposal.read"} {
		operation, ok := operations[id]
		if !ok || operation.policy != "read_only" || operation.scopes != 0 {
			t.Errorf("tidy operation %q = %+v, %t", id, operation, ok)
		}
	}
	for _, id := range []string{"tidy.survey", "tidy.apply_selection"} {
		operation, ok := operations[id]
		if !ok || operation.policy != "reversible" || operation.scopes != 0 {
			t.Errorf("tidy brokered operation %q = %+v, %t", id, operation, ok)
		}
	}
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
	if manifest.Name != "reaper-plugin" || manifest.Version != "0.9.0" || len(manifest.Capabilities) != 1 {
		t.Fatalf("manifest = %+v", manifest)
	}
	operations := map[string]bool{}
	for _, id := range manifest.Capabilities[0].AgentOperations {
		operations[id] = true
	}
	for _, required := range []string{"state.read", "actions.list", "actions.run_safe", "actions.run_confirmed", "scripts.list", "draft.validate", "draft.run", "tidy.survey", "tidy.apply_selection"} {
		if !operations[required] {
			t.Errorf("agent operation %q is not declared", required)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatalf("private service must not be exposed as portable native MCP: %v", err)
	}
}

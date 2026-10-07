package reaper

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type contributionManifest struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	Capabilities []struct {
		ID              string   `json:"id"`
		AgentOperations []string `json:"agent_operations"`
	} `json:"capabilities"`
	Services []struct {
		ID         string `json:"id"`
		Operations []struct {
			ID             string          `json:"id"`
			Policy         string          `json:"policy"`
			Scopes         []string        `json:"scopes"`
			TimeoutClass   string          `json:"timeout_class"`
			MaxOutputBytes int             `json:"max_output_bytes"`
			InputSchema    json.RawMessage `json:"input_schema"`
			OutputSchema   json.RawMessage `json:"output_schema"`
		} `json:"operations"`
	} `json:"services"`
	RequiresHostFeatures []string `json:"requires_host_features"`
	HomeProfileFacts     *struct {
		ServiceID string `json:"service_id"`
		Operation string `json:"operation"`
	} `json:"home_profile_facts"`
}

// A host finds the profile read through the manifest, not by knowing this
// plugin: `home_profile_facts` must name a read-only operation of the declared
// service, behind the host feature that introduced the key, so an older host
// refuses the whole contribution instead of ignoring it.
func TestContributionNamesItsReadOnlyProfileFactsOperation(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".ori-plugin", "plugin.json")) // #nosec G304 -- fixed repository manifest
	if err != nil {
		t.Fatal(err)
	}
	var manifest contributionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	facts := manifest.HomeProfileFacts
	if facts == nil || len(manifest.Services) != 1 || facts.ServiceID != manifest.Services[0].ID || facts.Operation != "profile.read" {
		t.Fatalf("home_profile_facts = %+v", facts)
	}
	required := false
	for _, feature := range manifest.RequiresHostFeatures {
		required = required || feature == "home_profile_v1"
	}
	if !required {
		t.Fatalf("host features = %v, want home_profile_v1", manifest.RequiresHostFeatures)
	}
	declared := 0
	for _, operation := range manifest.Services[0].Operations {
		if operation.ID != facts.Operation {
			continue
		}
		declared++
		if operation.Policy != "read_only" || len(operation.Scopes) != 0 || operation.TimeoutClass != "fast" ||
			operation.MaxOutputBytes < 1 || operation.MaxOutputBytes > 65536 {
			t.Fatalf("profile.read = %+v", operation)
		}
		// One boolean in; closed facts out. Every key the service can answer
		// is declared, and every key the schema requires is always answered.
		// (The bounds are held to the schema's by
		// TestProfileBoundsAreTheOnesTheManifestDeclares.)
		var input struct {
			Properties           map[string]json.RawMessage `json:"properties"`
			Required             []string                   `json:"required"`
			AdditionalProperties *bool                      `json:"additionalProperties"`
		}
		if err := json.Unmarshal(operation.InputSchema, &input); err != nil {
			t.Fatal(err)
		}
		if len(input.Properties) != 1 || len(input.Required) != 1 || input.Required[0] != "include_templates" ||
			input.AdditionalProperties == nil || *input.AdditionalProperties {
			t.Fatalf("profile.read input schema = %s", operation.InputSchema)
		}
		var output struct {
			Properties           map[string]json.RawMessage `json:"properties"`
			Required             []string                   `json:"required"`
			AdditionalProperties *bool                      `json:"additionalProperties"`
		}
		if err := json.Unmarshal(operation.OutputSchema, &output); err != nil {
			t.Fatal(err)
		}
		if output.AdditionalProperties == nil || *output.AdditionalProperties {
			t.Fatalf("profile.read output schema is open: %s", operation.OutputSchema)
		}
		answer, err := json.Marshal((*Service)(nil).Profile(context.Background(), ProfileInput{}))
		if err != nil {
			t.Fatal(err)
		}
		var keys map[string]json.RawMessage
		if err := json.Unmarshal(answer, &keys); err != nil {
			t.Fatal(err)
		}
		for key := range keys {
			if _, ok := output.Properties[key]; !ok {
				t.Errorf("the service answers %q, which the output schema does not declare", key)
			}
		}
		for _, key := range output.Required {
			if _, ok := keys[key]; !ok {
				t.Errorf("the output schema requires %q, which the service does not always answer", key)
			}
		}
		// Every field the service can answer is declared, including the
		// optional ones.
		full, err := json.Marshal(ProfileResult{App: "REAPER", Installed: true, Version: "7.28", TemplatesAvailable: true,
			Templates: []ProfileTemplate{{Name: "A", Kind: ProfileTemplateProject, File: "A.RPP", ModifiedAt: "2026-10-01T10:00:00Z"}}})
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(full, &keys); err != nil {
			t.Fatal(err)
		}
		for key := range keys {
			if _, ok := output.Properties[key]; !ok {
				t.Errorf("the service can answer %q, which the output schema does not declare", key)
			}
		}
	}
	if declared != 1 {
		t.Fatalf("profile.read is declared %d times", declared)
	}
}

// The service cuts its answer to bounds compiled into it; a host checks the
// answer against the bounds the manifest declares and refuses it whole when
// one is exceeded. The two sets must be the same numbers, or a legal answer of
// the service could be an illegal one for the host.
func TestProfileBoundsAreTheOnesTheManifestDeclares(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".ori-plugin", "plugin.json")) // #nosec G304 -- fixed repository manifest
	if err != nil {
		t.Fatal(err)
	}
	var manifest contributionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	type text struct {
		MinLength int      `json:"minLength"`
		MaxLength int      `json:"maxLength"`
		Enum      []string `json:"enum"`
	}
	var schema struct {
		Properties struct {
			App       text `json:"app"`
			Version   text `json:"version"`
			Templates struct {
				MaxItems int `json:"maxItems"`
				Items    struct {
					Properties struct {
						Name       text `json:"name"`
						Kind       text `json:"kind"`
						File       text `json:"file"`
						ModifiedAt text `json:"modified_at"`
					} `json:"properties"`
					Required []string `json:"required"`
				} `json:"items"`
			} `json:"templates"`
		} `json:"properties"`
	}
	found := false
	for _, operation := range manifest.Services[0].Operations {
		if operation.ID != "profile.read" {
			continue
		}
		found = true
		if operation.MaxOutputBytes != maxProfileOutputBytes {
			t.Errorf("max_output_bytes = %d, the service cuts its answer to %d", operation.MaxOutputBytes, maxProfileOutputBytes)
		}
		if err := json.Unmarshal(operation.OutputSchema, &schema); err != nil {
			t.Fatal(err)
		}
	}
	if !found {
		t.Fatal("profile.read is not declared")
	}
	item := schema.Properties.Templates.Items.Properties
	for name, pair := range map[string][2]int{
		"templates maxItems":    {schema.Properties.Templates.MaxItems, maxProfileTemplates},
		"name maxLength":        {item.Name.MaxLength, maxProfileTemplateName},
		"file maxLength":        {item.File.MaxLength, maxProfileTemplateFile},
		"modified_at maxLength": {item.ModifiedAt.MaxLength, maxProfileModifiedAt},
		"version maxLength":     {schema.Properties.Version.MaxLength, maxProfileVersionLength},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s: the manifest declares %d, the service uses %d", name, pair[0], pair[1])
		}
	}
	if len(profileAppName) < schema.Properties.App.MinLength || len(profileAppName) > schema.Properties.App.MaxLength {
		t.Errorf("app %q does not fit the declared %d to %d", profileAppName, schema.Properties.App.MinLength, schema.Properties.App.MaxLength)
	}
	if len(item.Kind.Enum) != 2 || item.Kind.Enum[0] != ProfileTemplateProject || item.Kind.Enum[1] != ProfileTemplateTrack {
		t.Errorf("declared kinds = %v", item.Kind.Enum)
	}
	// A template the service lists always has the three required parts, and
	// the time it writes fits the declared length.
	if got := strings.Join(schema.Properties.Templates.Items.Required, ","); got != "name,kind,file" {
		t.Errorf("required template parts = %s", got)
	}
	if written := len(time.Date(9998, 12, 31, 23, 59, 59, 0, time.UTC).Format(time.RFC3339)); written > maxProfileModifiedAt {
		t.Errorf("a written time is %d characters, the bound is %d", written, maxProfileModifiedAt)
	}
	if profileOutputReserve < 256 || profileOutputReserve >= maxProfileOutputBytes/2 {
		t.Errorf("the reserve kept free of templates is %d of %d bytes", profileOutputReserve, maxProfileOutputBytes)
	}
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
	if manifest.Name != "reaper-plugin" || manifest.Version != "0.10.0" || len(manifest.Capabilities) != 1 {
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

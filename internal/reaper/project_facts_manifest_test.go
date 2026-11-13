package reaper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"
)

type projectFactsManifest struct {
	Capabilities []struct {
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
	ProjectFacts         *struct {
		ServiceID string   `json:"service_id"`
		Operation string   `json:"operation"`
		Formats   []string `json:"formats"`
	} `json:"project_facts"`
}

// schemaNode is the part of a JSON Schema the manifest uses.
type schemaNode struct {
	Type                 string                `json:"type"`
	Properties           map[string]schemaNode `json:"properties"`
	Required             []string              `json:"required"`
	AdditionalProperties *bool                 `json:"additionalProperties"`
	Items                *schemaNode           `json:"items"`
	MinItems             *int                  `json:"minItems"`
	MaxItems             *int                  `json:"maxItems"`
	MinLength            *int                  `json:"minLength"`
	MaxLength            *int                  `json:"maxLength"`
	Minimum              *float64              `json:"minimum"`
	Maximum              *float64              `json:"maximum"`
	Enum                 []string              `json:"enum"`
}

// closed reports every object in the schema that allows other properties.
func (n schemaNode) closed(t *testing.T, path string) {
	t.Helper()
	if n.Type == "object" && (n.AdditionalProperties == nil || *n.AdditionalProperties) {
		t.Errorf("%s allows other properties", path)
	}
	for name, child := range n.Properties {
		child.closed(t, path+"."+name)
	}
	if n.Items != nil {
		n.Items.closed(t, path+"[]")
	}
}

func readProjectFactsManifest(t *testing.T) projectFactsManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".ori-plugin", "plugin.json")) // #nosec G304 -- fixed repository manifest
	if err != nil {
		t.Fatal(err)
	}
	var manifest projectFactsManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

// A host finds the project reader through the manifest: `project_facts` names
// a read-only operation of the declared service and the formats it covers,
// behind the host feature that introduced the key, so an older host refuses
// the whole contribution instead of ignoring it. It is the eighth required
// feature, the most a host accepts.
func TestContributionNamesItsReadOnlyProjectFactsOperation(t *testing.T) {
	manifest := readProjectFactsManifest(t)
	facts := manifest.ProjectFacts
	if facts == nil || len(manifest.Services) != 1 || facts.ServiceID != manifest.Services[0].ID || facts.ServiceID != "reaper-service" ||
		facts.Operation != "project.facts.read" || !slices.Equal(facts.Formats, []string{ProjectFactsFormat}) {
		t.Fatalf("project_facts = %+v", facts)
	}
	if !slices.Contains(manifest.RequiresHostFeatures, "project_facts_v1") || len(manifest.RequiresHostFeatures) != 8 {
		t.Fatalf("host features = %v, want project_facts_v1 as the eighth", manifest.RequiresHostFeatures)
	}
	// The host calls it itself; no agent is offered it.
	for _, capability := range manifest.Capabilities {
		if slices.Contains(capability.AgentOperations, facts.Operation) {
			t.Fatalf("%s is offered to agents", facts.Operation)
		}
	}
	declared := 0
	for _, operation := range manifest.Services[0].Operations {
		if operation.ID != facts.Operation {
			continue
		}
		declared++
		if operation.Policy != "read_only" || operation.Scopes == nil || len(operation.Scopes) != 0 || operation.TimeoutClass != "long" ||
			operation.MaxOutputBytes != maxProjectFactsOutputBytes {
			t.Fatalf("project.facts.read = %+v", operation)
		}
	}
	if declared != 1 {
		t.Fatalf("project.facts.read is declared %d times", declared)
	}
}

func projectFactsSchemas(t *testing.T) (schemaNode, schemaNode) {
	t.Helper()
	manifest := readProjectFactsManifest(t)
	for _, operation := range manifest.Services[0].Operations {
		if operation.ID != "project.facts.read" {
			continue
		}
		var input, output schemaNode
		if err := json.Unmarshal(operation.InputSchema, &input); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(operation.OutputSchema, &output); err != nil {
			t.Fatal(err)
		}
		return input, output
	}
	t.Fatal("project.facts.read is not declared")
	return schemaNode{}, schemaNode{}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func intBound(t *testing.T, name string, got *int, want int) {
	t.Helper()
	if got == nil || *got != want {
		t.Errorf("%s: the manifest declares %v, the service uses %d", name, got, want)
	}
}

func numberRange(t *testing.T, name string, node schemaNode, low, high float64) {
	t.Helper()
	if node.Minimum == nil || node.Maximum == nil || *node.Minimum != low || *node.Maximum != high {
		t.Errorf("%s: the manifest declares %v to %v, the service uses %v to %v", name, node.Minimum, node.Maximum, low, high)
	}
}

// The service checks its input and cuts its answer to bounds compiled into
// it; a host checks both against the bounds the manifest declares. The two
// sets must be the same numbers, every object closed, and every key the
// service can answer declared.
func TestProjectFactsBoundsAreTheOnesTheManifestDeclares(t *testing.T) {
	input, output := projectFactsSchemas(t)
	input.closed(t, "input")
	output.closed(t, "output")

	if got := sortedKeys(input.Properties); !slices.Equal(got, []string{"files", "max_file_bytes", "names", "schema_version"}) ||
		!slices.Equal(input.Required, []string{"schema_version", "names", "max_file_bytes", "files"}) {
		t.Fatalf("input = %v, required %v", got, input.Required)
	}
	numberRange(t, "schema_version", input.Properties["schema_version"], ProjectFactsSchemaVersion, ProjectFactsSchemaVersion)
	numberRange(t, "max_file_bytes", input.Properties["max_file_bytes"], 1, maxProjectFactsFileBytes)
	files := input.Properties["files"]
	intBound(t, "files minItems", files.MinItems, 1)
	intBound(t, "files maxItems", files.MaxItems, maxProjectFactsFiles)
	file := *files.Items
	if !slices.Equal(file.Required, []string{"id", "format", "path", "size"}) || len(file.Properties) != 4 {
		t.Fatalf("input file = %+v", file)
	}
	numberRange(t, "file id", file.Properties["id"], 0, maxProjectFactsFileID)
	intBound(t, "format maxLength", file.Properties["format"].MaxLength, maxProjectFactsFormatChars)
	intBound(t, "path maxLength", file.Properties["path"].MaxLength, maxProjectFactsPathChars)
	if size := file.Properties["size"]; size.Type != "integer" || size.Minimum == nil || *size.Minimum != 0 {
		t.Errorf("size = %+v", size)
	}

	if !slices.Equal(output.Required, []string{"schema_version", "files"}) {
		t.Errorf("output required = %v", output.Required)
	}
	answers := output.Properties["files"]
	intBound(t, "answers minItems", answers.MinItems, 1)
	intBound(t, "answers maxItems", answers.MaxItems, maxProjectFactsFiles)
	entry := *answers.Items
	if !slices.Equal(entry.Required, []string{"id", "result"}) {
		t.Errorf("entry required = %v", entry.Required)
	}
	if got := entry.Properties["result"].Enum; !slices.Equal(got, []string{ProjectFactsRead, ProjectFactsTooLarge, ProjectFactsUnreadable, ProjectFactsFailed}) {
		t.Errorf("results = %v", got)
	}
	numberRange(t, "tempo_bpm", entry.Properties["tempo_bpm"], 0, maxProjectFactsTempo)
	numberRange(t, "length_seconds", entry.Properties["length_seconds"], 0, maxProjectFactsLength)
	numberRange(t, "track_count", entry.Properties["track_count"], 0, maxProjectFactsTracks)
	numberRange(t, "tracks_left_out", entry.Properties["tracks_left_out"], 0, maxProjectFactsLeftOut)
	numberRange(t, "markers_left_out", entry.Properties["markers_left_out"], 0, maxProjectFactsLeftOut)
	tracks := entry.Properties["tracks"]
	intBound(t, "tracks maxItems", tracks.MaxItems, maxProjectFactsNames)
	intBound(t, "track name minLength", tracks.Items.MinLength, 1)
	intBound(t, "track name maxLength", tracks.Items.MaxLength, maxProjectFactsNameRunes)
	markers := entry.Properties["markers"]
	intBound(t, "markers maxItems", markers.MaxItems, maxProjectFactsNames)
	marker := *markers.Items
	intBound(t, "marker name maxLength", marker.Properties["name"].MaxLength, maxProjectFactsNameRunes)
	numberRange(t, "marker at", marker.Properties["at"], 0, maxProjectFactsLength)
	numberRange(t, "marker end", marker.Properties["end"], 0, maxProjectFactsLength)
	if !slices.Equal(marker.Required, []string{"at"}) {
		t.Errorf("marker required = %v", marker.Required)
	}

	// Every key a full answer carries is declared, and nothing else is.
	full := ProjectFactsResult{SchemaVersion: 1, Files: []ProjectFactsEntry{readProjectFactsFileForSchema(t)}}
	encoded, err := json.Marshal(full)
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Files []map[string]json.RawMessage `json:"files"`
	}
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	if got, want := sortedKeys(raw.Files[0]), sortedKeys(entry.Properties); !slices.Equal(got, want) {
		t.Errorf("a full answer carries %v, the schema declares %v", got, want)
	}
	var regions []map[string]json.RawMessage
	if err := json.Unmarshal(raw.Files[0]["markers"], &regions); err != nil {
		t.Fatal(err)
	}
	if got, want := sortedKeys(regions[1]), sortedKeys(marker.Properties); !slices.Equal(got, want) {
		t.Errorf("a region carries %v, the schema declares %v", got, want)
	}
}

func readProjectFactsFileForSchema(t *testing.T) ProjectFactsEntry {
	t.Helper()
	path, size := writeProjectFile(t, t.TempDir(), "Song.rpp", realShapeProject)
	entry := readProjectFactsFile(ProjectFactsFileSpec{ID: 0, Format: ProjectFactsFormat, Path: path, Size: size}, maxProjectFactsFileBytes, true)
	if entry.Result != ProjectFactsRead || len(entry.Markers) != 2 || entry.Markers[1].End == 0 {
		t.Fatalf("entry = %+v", entry)
	}
	return entry
}

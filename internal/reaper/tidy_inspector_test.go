package reaper

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func canonicalTidyInspector(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "skills", "reaper-project-tidy", "scripts", "inspect_project.lua")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read canonical inspector: %v", err)
	}
	return string(data)
}

func TestCanonicalTidyInspectorPassesReadOnlyRunnerAudit(t *testing.T) {
	script := canonicalTidyInspector(t)
	if !strings.HasPrefix(script, readOnlyInspectorHeader) {
		t.Fatalf("canonical inspector does not select the reserved read-only mode")
	}
	if err := validateReadOnlyInspectorLua(script); err != nil {
		t.Fatalf("canonical inspector audit error = %v", err)
	}
	for _, forbidden := range []string{
		"reaper.Undo_",
		"reaper.Set",
		"reaper.Insert",
		"reaper.Delete",
		"reaper.Main_",
		"reaper.TrackList_AdjustWindows",
		"reaper.UpdateArrange",
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("canonical inspector contains mutating API family %q", forbidden)
		}
	}
}

func TestCanonicalTidyInspectorAuditRejectsInjectedMutation(t *testing.T) {
	script := canonicalTidyInspector(t)
	mutated := script + "\nreaper.SetMediaTrackInfo_Value(reaper.GetTrack(0, 0), 'B_MUTE', 1)\n"
	if err := validateReadOnlyInspectorLua(mutated); !errors.Is(err, ErrReadOnlyInspectionRejected) {
		t.Fatalf("mutating inspector audit error = %v", err)
	}
}

func TestCanonicalTidyInspectorLiveEvidenceKeepsProjectCountUnchanged(t *testing.T) {
	root := filepath.Join("..", "..", "scripts", "spikes", "reaper-project-tidy")
	var counts []int64
	for _, filename := range []string{"observed-inspector-state.json", "observed-inspector-repeat.json"} {
		data, err := os.ReadFile(filepath.Join(root, filename))
		if err != nil {
			t.Fatalf("read live inspector evidence %s: %v", filename, err)
		}
		state, err := DecodeTidyInspectedState(data)
		if err != nil {
			t.Fatalf("decode live inspector evidence %s: %v", filename, err)
		}
		counts = append(counts, state.Project.ProjectChangeCount)
	}
	if counts[0] != counts[1] {
		t.Fatalf("read-only inspection changed project count: %v", counts)
	}
}

func TestCanonicalTidyInspectorGuardsUnchangedProjectCountAndAtomicBoundedOutput(t *testing.T) {
	script := canonicalTidyInspector(t)
	if count := strings.Count(script, "reaper.GetProjectStateChangeCount(project)"); count != 2 {
		t.Fatalf("project state-change count reads = %d, want 2", count)
	}
	for _, required := range []string{
		"if final_change_count ~= project_change_count then",
		"project changed during inspection",
		"local MAX_OUTPUT_BYTES = 4 * 1024 * 1024",
		"if output_bytes > MAX_OUTPUT_BYTES then",
		"local temp_path = home .. \"/.ori-reaper/.state.json.tmp-\"",
		"os.rename(temp_path, state_path)",
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("canonical inspector is missing %q", required)
		}
	}
	if strings.Index(script, "file:close()") > strings.Index(script, "os.rename(temp_path, state_path)") {
		t.Fatalf("canonical inspector renames before closing the temporary file")
	}
}

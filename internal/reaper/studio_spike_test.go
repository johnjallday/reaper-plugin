package reaper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The spike probes under scripts/spikes/reaper-studio are kept honest against
// the current audit: both pass the 0.11.0 allowlist, which added
// EnumInstalledFX. The findings record that the installed 0.10.1 runner
// refused the same probe with the status the catalog maps to reader_outdated.
func TestSpikeProbesPassTheCurrentAuditAndRecordTheOldRefusal(t *testing.T) {
	root := filepath.Join("..", "..", "scripts", "spikes", "reaper-studio")
	for _, name := range []string{"state-probe.lua", "fx-audit-probe.lua"} {
		data, err := os.ReadFile(filepath.Join(root, name)) // #nosec G304 -- fixed spike fixtures
		if err != nil {
			t.Fatal(err)
		}
		if err := validateReadOnlyInspectorLua(string(data)); err != nil {
			t.Fatalf("%s audit: %v", name, err)
		}
	}
	findings, err := os.ReadFile(filepath.Join(root, "findings.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{
		"error: read-only audit: API EnumInstalledFX is not read-only",
		"audio_plugins` has `access: \"live\"`",
		"app_closed",
	} {
		if !strings.Contains(string(findings), phrase) {
			t.Errorf("findings do not record %q", phrase)
		}
	}
}

package reaper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSpikeStateProbePassesTheInstalledAudit(t *testing.T) {
	root := filepath.Join("..", "..", "scripts", "spikes", "reaper-studio")
	state, err := os.ReadFile(filepath.Join(root, "state-probe.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateReadOnlyInspectorLua(string(state)); err != nil {
		t.Fatalf("state probe audit: %v", err)
	}
	audit, err := os.ReadFile(filepath.Join(root, "fx-audit-probe.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateReadOnlyInspectorLua(string(audit)); err == nil {
		t.Fatal("fx audit probe should be rejected by the 0.10.1 allowlist")
	}
}

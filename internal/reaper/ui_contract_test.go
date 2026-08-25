package reaper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceSurfaceUIUsesOnlySDKAndDeclaredOperations(t *testing.T) {
	root := filepath.Join("..", "..", "ui", "live-control")
	for _, name := range []string{"index.html", "style.css", "app.js", "workspace-surface-sdk.js"} {
		if info, err := os.Stat(filepath.Join(root, name)); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("UI asset %s: %+v, %v", name, info, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "app.js")) // #nosec G304 -- fixed repository fixture
	if err != nil {
		t.Fatal(err)
	}
	model, err := os.ReadFile(filepath.Join(root, "model.js")) // #nosec G304 -- fixed repository fixture
	if err != nil {
		t.Fatal(err)
	}
	source := string(data) + string(model)
	for _, operation := range []string{"state.read", "actions.list", "actions.run_safe", "actions.run_confirmed", "actions.run_raw_confirmed", "scripts.list", "draft.validate", "draft.run"} {
		if !strings.Contains(source, operation) {
			t.Errorf("UI does not use declared operation %q", operation)
		}
	}
	for _, forbidden := range []string{"innerHTML", "parent.document", "window.parent.", "fetch("} {
		if strings.Contains(source, forbidden) {
			t.Errorf("sandboxed UI contains forbidden ambient access %q", forbidden)
		}
	}
}

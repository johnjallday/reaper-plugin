package reaper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServiceHomeOverridePreservesHomeUntilOperatorPathIsValidated(t *testing.T) {
	original := t.TempDir()
	t.Setenv("HOME", original)
	t.Setenv(envServiceHome, "")
	if err := ApplyServiceHomeOverride(); err != nil || os.Getenv("HOME") != original {
		t.Fatalf("empty override changed HOME: %v", err)
	}

	target := t.TempDir()
	t.Setenv(envServiceHome, target)
	if err := ApplyServiceHomeOverride(); err != nil || os.Getenv("HOME") != filepath.Clean(target) {
		t.Fatalf("explicit directory override was not applied: %v", err)
	}

	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "linked-home")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, path string }{
		{"missing", filepath.Join(t.TempDir(), "missing")},
		{"file", file},
		{"symlink", link},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", original)
			t.Setenv(envServiceHome, tc.path)
			if err := ApplyServiceHomeOverride(); err == nil {
				t.Fatal("unsafe operator path accepted")
			}
			if os.Getenv("HOME") != original {
				t.Fatal("rejected override changed HOME")
			}
		})
	}
}

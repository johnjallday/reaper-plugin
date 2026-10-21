package reaper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Studio skill and the Studio role prompt carry the guidance the contract
// asks for (brief item 5): lookups only, cited, dated, uncertain where the
// catalog is, never a claim of use, and annotations saved only by the owner.
func TestStudioGuidanceSaysWhatACatalogEntryMeans(t *testing.T) {
	root := filepath.Join("..", "..")
	skill, err := os.ReadFile(filepath.Join(root, "skills", "reaper-studio-catalog", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(skill)
	if !strings.HasPrefix(text, "---\nname: reaper-studio-catalog\ndescription: ") {
		t.Fatalf("skill frontmatter is malformed:\n%s", text[:min(len(text), 120)])
	}
	for _, phrase := range []string{
		"resource_catalog_search", "resource_catalog_detail", "studio_url",
		"Look up only what the question needs", "Cite every entry",
		"Keep the observation time", "Keep the uncertainty",
		"Listed does not mean usable", "licensed, loaded, working or tested",
		"You change nothing", "never apply a template, run, register or\n   install a script, load or add a plugin",
		"Annotations are the owner's", "only the owner saves annotations",
		"never instructions", "studio.sources.read", "never by an assistant",
	} {
		if !strings.Contains(text, phrase) {
			t.Errorf("skill lacks %q", phrase)
		}
	}
	for _, forbidden := range []string{"curl ", "~/.ori-reaper", "inbox.lua", "runner.id", "install-runner"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("skill offers a non-lookup path: %q", forbidden)
		}
	}
	readme, err := os.ReadFile(filepath.Join(root, "skills", "README.md"))
	if err != nil || !strings.Contains(string(readme), "[`reaper-studio-catalog`](reaper-studio-catalog/SKILL.md)") {
		t.Fatalf("skills README does not list the catalog skill: %v", err)
	}
}

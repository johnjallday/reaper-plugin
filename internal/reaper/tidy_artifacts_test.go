package reaper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTidyArtifactStoreWritesAtomicProposalAndSupersedesOlderOpenProposal(t *testing.T) {
	root := t.TempDir()
	store, err := NewTidyArtifactStore(root)
	if err != nil {
		t.Fatal(err)
	}
	conventions, err := ParseTidyConventions(DefaultTidyConventions())
	if err != nil {
		t.Fatal(err)
	}
	firstProposal, err := GenerateTidySurvey(messyTidySurveyState(), conventions, "survey-001", "Created conventions.md from defaults.")
	if err != nil {
		t.Fatal(err)
	}
	firstTime := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	first, err := store.WriteProposal(firstProposal, firstTime)
	if err != nil {
		t.Fatal(err)
	}
	if first.Record.Status != TidyProposalStatusOpen || first.Plan == nil ||
		!strings.Contains(first.Summary, "Nothing in REAPER has been changed") ||
		!strings.Contains(first.Summary, "Created conventions.md") {
		t.Fatalf("first stored proposal = %+v", first)
	}
	assertTidyArtifactFiles(t, root, first.Record)

	secondProposal, err := GenerateTidySurvey(messyTidySurveyState(), conventions, "survey-002", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.WriteProposal(secondProposal, firstTime.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if second.Record.Status != TidyProposalStatusOpen {
		t.Fatalf("second status = %s", second.Record.Status)
	}
	firstAfter, err := store.ReadProposal("survey-001")
	if err != nil {
		t.Fatal(err)
	}
	if firstAfter.Record.Status != TidyProposalStatusSuperseded || firstAfter.Record.SupersededBy != "survey-002" {
		t.Fatalf("first supersession = %+v", firstAfter.Record)
	}
	latest, found, err := store.LatestProposal()
	if err != nil || !found || latest.Record.ProposalID != "survey-002" {
		t.Fatalf("latest = %+v, %t, %v", latest, found, err)
	}
	open, found, err := store.OpenProposal()
	if err != nil || !found || open.Record.ProposalID != "survey-002" {
		t.Fatalf("open = %+v, %t, %v", open, found, err)
	}
	if matches, _ := filepath.Glob(filepath.Join(root, "tidy", ".ori-tidy-*")); len(matches) != 0 {
		t.Fatalf("atomic temp artifacts remain: %v", matches)
	}
}

func TestTidyArtifactStorePersistsAlreadyTidyAndDismissedLifecycle(t *testing.T) {
	root := t.TempDir()
	store, err := NewTidyArtifactStore(root)
	if err != nil {
		t.Fatal(err)
	}
	conventions, _ := ParseTidyConventions(DefaultTidyConventions())
	proposal, _ := GenerateTidySurvey(messyTidySurveyState(), conventions, "survey-open", "")
	created := time.Date(2026, 8, 28, 11, 0, 0, 0, time.UTC)
	if _, err := store.WriteProposal(proposal, created); err != nil {
		t.Fatal(err)
	}
	if err := store.DismissProposal("survey-open"); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.OpenProposal(); err != nil || found {
		t.Fatalf("dismissed proposal remained open: %t, %v", found, err)
	}
	dismissed, err := store.ReadProposal("survey-open")
	if err != nil || dismissed.Record.Status != TidyProposalStatusDismissed {
		t.Fatalf("dismissed = %+v, %v", dismissed, err)
	}

	already := TidySurveyProposal{ProposalID: "survey-tidy", AlreadyTidy: true, Lines: []TidyProposalLine{}}
	stored, err := store.WriteProposal(already, created.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if stored.Record.Status != TidyProposalStatusAlreadyTidy || stored.Plan != nil ||
		!strings.Contains(stored.Summary, "Project already tidy") {
		t.Fatalf("already tidy = %+v", stored)
	}
	if _, err := os.Stat(filepath.Join(root, "tidy", tidyPlanFileName("survey-tidy"))); !os.IsNotExist(err) {
		t.Fatalf("already-tidy outcome wrote an edit plan: %v", err)
	}
}

func TestTidyArtifactStoreRejectsUnsupportedProposalBeforeVisibility(t *testing.T) {
	root := t.TempDir()
	store, _ := NewTidyArtifactStore(root)
	conventions, _ := ParseTidyConventions(DefaultTidyConventions())
	proposal, _ := GenerateTidySurvey(messyTidySurveyState(), conventions, "survey-invalid", "")
	proposal.Plan.Items[0].Verb = "set_track_volume"
	if _, err := store.WriteProposal(proposal, time.Now().UTC()); err == nil {
		t.Fatalf("unsupported proposal became visible")
	}
	matches, err := filepath.Glob(filepath.Join(root, "tidy", "proposal-*.state.json"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("invalid proposal left a commit record: %v, %v", matches, err)
	}
}

func TestTidyArtifactStoreRejectsNonMonotonicAndTamperedArtifacts(t *testing.T) {
	root := t.TempDir()
	store, _ := NewTidyArtifactStore(root)
	conventions, _ := ParseTidyConventions(DefaultTidyConventions())
	first, _ := GenerateTidySurvey(messyTidySurveyState(), conventions, "survey-z", "")
	created := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	if _, err := store.WriteProposal(first, created); err != nil {
		t.Fatal(err)
	}
	older, _ := GenerateTidySurvey(messyTidySurveyState(), conventions, "survey-a", "")
	if _, err := store.WriteProposal(older, created); err == nil {
		t.Fatalf("non-monotonic proposal identity was accepted")
	}

	planPath := filepath.Join(root, "tidy", tidyPlanFileName("survey-z"))
	if err := os.Remove(planPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside.json"), planPath); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadProposal("survey-z"); err == nil {
		t.Fatalf("symlinked plan artifact was accepted")
	}
}

func assertTidyArtifactFiles(t *testing.T, root string, record TidyProposalRecord) {
	t.Helper()
	for _, name := range []string{
		"proposal-" + record.ProposalID + ".state.json",
		record.PlanFile,
		record.SummaryFile,
	} {
		info, err := os.Lstat(filepath.Join(root, "tidy", name))
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
			t.Fatalf("artifact %s = %+v, %v", name, info, err)
		}
	}
}

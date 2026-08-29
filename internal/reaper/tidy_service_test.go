package reaper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTidyServiceProjectsLatestProposalWithoutPathsOrRawPlan(t *testing.T) {
	host, store := tidyServiceFixture(t)
	service := &Service{}
	status, err := service.TidyStatus(host)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != TidyProposalStatusOpen || !status.HasOpenProposal || status.ProposalID != "surface-001" || status.ItemCount == 0 {
		t.Fatalf("status = %+v", status)
	}
	view, err := service.ReadTidyProposal(host, TidyProposalReadInput{})
	if err != nil {
		t.Fatal(err)
	}
	if view.ProposalID != "surface-001" || !view.HasOpenProposal || len(view.Items) != status.ItemCount {
		t.Fatalf("view = %+v", view)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), host.WorkspaceRoot) || strings.Contains(string(encoded), "plan_file") || strings.Contains(string(encoded), "track_guid") {
		t.Fatalf("public tidy projection exposed private path/raw plan: %s", encoded)
	}

	dismissed, err := service.DismissTidyProposal(host, TidyProposalActionInput{ProposalID: "surface-001"})
	if err != nil || dismissed.Outcome != "dismissed" || !strings.Contains(dismissed.Summary, "without changing REAPER") {
		t.Fatalf("dismiss = %+v, %v", dismissed, err)
	}
	status, err = service.TidyStatus(host)
	if err != nil || status.HasOpenProposal || status.State != TidyProposalStatusDismissed {
		t.Fatalf("dismissed status = %+v, %v", status, err)
	}

	stored, err := store.ReadProposal("surface-001")
	if err != nil || stored.Plan == nil {
		t.Fatalf("private artifact plan missing: %+v, %v", stored, err)
	}
}

func TestTidyServiceProjectsLastReportAndFreshSurveyRecommendation(t *testing.T) {
	host, store := tidyServiceFixture(t)
	stored, err := store.ReadProposal("surface-001")
	if err != nil {
		t.Fatal(err)
	}
	items := make([]TidyApplyResultItem, len(stored.Plan.Items))
	for index, item := range stored.Plan.Items {
		items[index] = TidyApplyResultItem{ID: item.ID, Verb: item.Verb, Status: TidyApplyStatusSkipped, Reason: "snapshot changed"}
	}
	result := TidyApplyResult{
		SchemaVersion: TidySchemaVersion, PlanID: stored.Plan.PlanID,
		ProjectChangeCountBefore: 20, ProjectChangeCountAfter: 20, Items: items,
	}
	if _, err := store.WriteApplyReport("surface-001", result, time.Date(2026, 8, 28, 12, 1, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	service := &Service{}
	view, err := service.ReadTidyProposal(host, TidyProposalReadInput{})
	if err != nil || view.LastReport == nil || !view.LastReport.FreshSurveyRecommended ||
		view.LastReport.UndoSummary != TidyUndoSentence || view.State != TidyProposalStatusApplySkipped {
		t.Fatalf("report view = %+v, %v", view, err)
	}
	status, err := service.TidyStatus(host)
	if err != nil || status.LastReportOutcome != TidyReportOutcomeFullySkipped || status.HasOpenProposal {
		t.Fatalf("report status = %+v, %v", status, err)
	}
}

func TestTidyServiceReturnsNoneWithoutCreatingArtifactDirectory(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "Song.rpp")
	if err := os.WriteFile(project, []byte("<REAPER_PROJECT\n>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	host := HostContext{WorkspaceID: "workspace-a", WorkspaceRoot: root, ProjectEntry: project}
	status, err := (&Service{}).TidyStatus(host)
	if err != nil || status.State != "none" {
		t.Fatalf("status = %+v, %v", status, err)
	}
	if _, err := os.Stat(filepath.Join(root, "tidy")); !os.IsNotExist(err) {
		t.Fatalf("read-only status created artifact state: %v", err)
	}
}

func TestTidyServiceRejectsTraversalSymlinksOversizeAndForeignRoots(t *testing.T) {
	host, _ := tidyServiceFixture(t)
	service := &Service{}
	if _, err := service.ReadTidyProposal(host, TidyProposalReadInput{ProposalID: "../surface-001"}); err == nil {
		t.Fatalf("traversal proposal ID was accepted")
	}

	foreignRoot := t.TempDir()
	foreign := host
	foreign.WorkspaceRoot = foreignRoot
	if _, err := service.TidyStatus(foreign); err == nil {
		t.Fatalf("project outside host workspace was accepted")
	}

	symlinkRoot := t.TempDir()
	symlinkProject := filepath.Join(symlinkRoot, "Song.rpp")
	if err := os.Symlink(host.ProjectEntry, symlinkProject); err != nil {
		t.Fatal(err)
	}
	if _, err := service.TidyStatus(HostContext{WorkspaceID: "workspace-b", WorkspaceRoot: symlinkRoot, ProjectEntry: symlinkProject}); err == nil {
		t.Fatalf("symlink project entry was accepted")
	}

	summary := filepath.Join(host.WorkspaceRoot, "tidy", tidySummaryFileName("surface-001"))
	if err := os.WriteFile(summary, []byte(strings.Repeat("x", maxTidyArtifactSummaryBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReadTidyProposal(host, TidyProposalReadInput{ProposalID: "surface-001"}); err == nil {
		t.Fatalf("oversized proposal summary was accepted")
	}
}

func tidyServiceFixture(t *testing.T) (HostContext, *TidyArtifactStore) {
	t.Helper()
	root := t.TempDir()
	project := filepath.Join(root, "Song.rpp")
	if err := os.WriteFile(project, []byte("<REAPER_PROJECT\n>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewTidyArtifactStore(root)
	if err != nil {
		t.Fatal(err)
	}
	conventions, _ := ParseTidyConventions(DefaultTidyConventions())
	proposal, err := GenerateTidySurvey(messyTidySurveyState(), conventions, "surface-001", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.WriteProposal(proposal, time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	return HostContext{WorkspaceID: "workspace-a", WorkspaceRoot: root, ProjectEntry: project}, store
}

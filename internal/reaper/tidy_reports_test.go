package reaper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTidyApplyReportPersistsRecoverableResultAndClosesProposal(t *testing.T) {
	root := t.TempDir()
	store, _ := NewTidyArtifactStore(root)
	plan := validTidyPlanFixture()
	proposal := tidyProposalFromPlan(plan)
	created := time.Date(2026, 8, 28, 13, 0, 0, 0, time.UTC)
	if _, err := store.WriteProposal(proposal, created); err != nil {
		t.Fatal(err)
	}
	result := validTidyResultFixture()
	report, err := store.WriteApplyReport(plan.PlanID, result, created.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if report.Record.Outcome != TidyReportOutcomePartial || !strings.Contains(report.Markdown, TidyUndoSentence) ||
		!strings.Contains(report.Markdown, "## Failed") || !strings.Contains(report.Markdown, "## Skipped") {
		t.Fatalf("report = %+v", report)
	}
	stored, err := store.ReadProposal(plan.PlanID)
	if err != nil || stored.Record.Status != TidyProposalStatusApplied {
		t.Fatalf("proposal status = %+v, %v", stored.Record, err)
	}
	if _, found, err := store.OpenProposal(); err != nil || found {
		t.Fatalf("applied proposal remained open: %t, %v", found, err)
	}
	latest, found, err := store.LatestApplyReport()
	if err != nil || !found || latest.Record.ProposalID != plan.PlanID || latest.Result.ValidateAgainstPlan(plan) != nil {
		t.Fatalf("latest report = %+v, %t, %v", latest, found, err)
	}
	for _, name := range []string{
		tidyApplyResultFileName(plan.PlanID), tidyApplyReportFileName(plan.PlanID), tidyApplyReportStateFileName(plan.PlanID),
	} {
		info, err := os.Lstat(filepath.Join(root, "tidy", name))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			t.Fatalf("report artifact %s = %+v, %v", name, info, err)
		}
	}
}

func TestTidyApplyReportAcceptsSelectedOnlyResult(t *testing.T) {
	root := t.TempDir()
	store, _ := NewTidyArtifactStore(root)
	plan := validTidyPlanFixture()
	created := time.Date(2026, 8, 28, 13, 30, 0, 0, time.UTC)
	if _, err := store.WriteProposal(tidyProposalFromPlan(plan), created); err != nil {
		t.Fatal(err)
	}
	result := validTidyResultFixture()
	result.Items = []TidyApplyResultItem{result.Items[1], result.Items[3]}
	report, err := store.WriteApplyReport(plan.PlanID, result, created.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Record.Items) != 2 || report.Record.Items[0].ID != plan.Items[1].ID || report.Record.Items[1].ID != plan.Items[3].ID {
		t.Fatalf("selected-only report = %+v", report.Record.Items)
	}
}

func TestTidyApplyReportTreatsFullySkippedAsFreshSurveyOutcome(t *testing.T) {
	root := t.TempDir()
	store, _ := NewTidyArtifactStore(root)
	plan := validTidyPlanFixture()
	created := time.Date(2026, 8, 28, 14, 0, 0, 0, time.UTC)
	if _, err := store.WriteProposal(tidyProposalFromPlan(plan), created); err != nil {
		t.Fatal(err)
	}
	items := make([]TidyApplyResultItem, len(plan.Items))
	for index, item := range plan.Items {
		items[index] = TidyApplyResultItem{ID: item.ID, Verb: item.Verb, Status: TidyApplyStatusSkipped, Reason: "snapshot changed"}
	}
	result := TidyApplyResult{
		SchemaVersion: TidySchemaVersion, PlanID: plan.PlanID,
		ProjectChangeCountBefore: 50, ProjectChangeCountAfter: 50, Items: items,
	}
	report, err := store.WriteApplyReport(plan.PlanID, result, created.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if report.Record.Outcome != TidyReportOutcomeFullySkipped || !strings.Contains(report.Markdown, "Run a fresh survey") {
		t.Fatalf("fully skipped report = %+v", report)
	}
	proposal, err := store.ReadProposal(plan.PlanID)
	if err != nil || proposal.Record.Status != TidyProposalStatusApplySkipped {
		t.Fatalf("fully skipped proposal = %+v, %v", proposal.Record, err)
	}
}

func TestTidyApplyReportRejectsInjectedOrMismatchedResultBeforeCommitRecord(t *testing.T) {
	root := t.TempDir()
	store, _ := NewTidyArtifactStore(root)
	plan := validTidyPlanFixture()
	created := time.Date(2026, 8, 28, 15, 0, 0, 0, time.UTC)
	if _, err := store.WriteProposal(tidyProposalFromPlan(plan), created); err != nil {
		t.Fatal(err)
	}
	result := validTidyResultFixture()
	result.Items[0].ID = "injected"
	if _, err := store.WriteApplyReport(plan.PlanID, result, created.Add(time.Minute)); err == nil {
		t.Fatalf("mismatched apply result was persisted")
	}
	if _, err := os.Stat(filepath.Join(root, "tidy", tidyApplyReportStateFileName(plan.PlanID))); !os.IsNotExist(err) {
		t.Fatalf("invalid result left a report commit record: %v", err)
	}
}

func tidyProposalFromPlan(plan TidyEditPlan) TidySurveyProposal {
	lines := make([]TidyProposalLine, len(plan.Items))
	for index, item := range plan.Items {
		lines[index] = TidyProposalLine{ItemID: item.ID, Line: "Reviewed change " + item.ID, Reason: item.Reason}
	}
	return TidySurveyProposal{ProposalID: plan.PlanID, Plan: &plan, Lines: lines}
}

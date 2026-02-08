package artifact

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewStore_UsesEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(ProjectDirEnv, dir)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if store.BaseDir() != dir {
		t.Errorf("BaseDir: expected %q, got %q", dir, store.BaseDir())
	}
}

func TestStore_ProjectDir_NormalizesName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(ProjectDirEnv, dir)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	got := store.ProjectDir("My Project")
	want := filepath.Join(dir, "my-project")
	if got != want {
		t.Errorf("ProjectDir: expected %q, got %q", want, got)
	}
}

func TestStore_Load_MissingProject(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(ProjectDirEnv, dir)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	artifacts := store.Load("nonexistent")
	if artifacts.Plan != "" || artifacts.Design != "" {
		t.Errorf("Load nonexistent: expected empty, got Plan=%q Design=%q", artifacts.Plan, artifacts.Design)
	}
}

func TestStore_Load_PlanOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(ProjectDirEnv, dir)

	projDir := filepath.Join(dir, "test-proj")
	_ = os.MkdirAll(projDir, 0755)
	_ = os.WriteFile(filepath.Join(projDir, "plan.md"), []byte("  # My Plan\n  First line.\n"), 0644)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	artifacts := store.Load("test-proj")
	if artifacts.Plan != "# My Plan\n  First line." {
		t.Errorf("Plan: expected trimmed content, got %q", artifacts.Plan)
	}
	if artifacts.Design != "" {
		t.Errorf("Design: expected empty, got %q", artifacts.Design)
	}
}

func TestStore_Load_DesignOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(ProjectDirEnv, dir)

	projDir := filepath.Join(dir, "test-proj")
	_ = os.MkdirAll(projDir, 0755)
	_ = os.WriteFile(filepath.Join(projDir, "design.md"), []byte("# Architecture\n"), 0644)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	artifacts := store.Load("test-proj")
	if artifacts.Plan != "" {
		t.Errorf("Plan: expected empty, got %q", artifacts.Plan)
	}
	if artifacts.Design != "# Architecture" {
		t.Errorf("Design: expected content, got %q", artifacts.Design)
	}
}

func TestStore_Load_Both(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(ProjectDirEnv, dir)

	projDir := filepath.Join(dir, "full-proj")
	_ = os.MkdirAll(projDir, 0755)
	_ = os.WriteFile(filepath.Join(projDir, "plan.md"), []byte("Plan content"), 0644)
	_ = os.WriteFile(filepath.Join(projDir, "design.md"), []byte("Design content"), 0644)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	artifacts := store.Load("full-proj")
	if artifacts.Plan != "Plan content" || artifacts.Design != "Design content" {
		t.Errorf("Load: got Plan=%q Design=%q", artifacts.Plan, artifacts.Design)
	}
}

func TestProjectArtifacts_PlanSummary_Empty(t *testing.T) {
	a := ProjectArtifacts{}
	if got := a.PlanSummary(); got != "no plan yet" {
		t.Errorf("PlanSummary empty: expected 'no plan yet', got %q", got)
	}
}

func TestProjectArtifacts_PlanSummary_Short(t *testing.T) {
	a := ProjectArtifacts{Plan: "Short plan title"}
	if got := a.PlanSummary(); got != "Short plan title" {
		t.Errorf("PlanSummary short: expected 'Short plan title', got %q", got)
	}
}

func TestProjectArtifacts_PlanSummary_Truncates(t *testing.T) {
	long := "A" + string(make([]byte, 70))
	a := ProjectArtifacts{Plan: long}
	got := a.PlanSummary()
	if len(got) != 60 || got[len(got)-3:] != "..." {
		t.Errorf("PlanSummary long: expected 60 chars + ..., got len=%d %q", len(got), got)
	}
}

func TestProjectArtifacts_DesignSummary_Empty(t *testing.T) {
	a := ProjectArtifacts{}
	if got := a.DesignSummary(); got != "no design yet" {
		t.Errorf("DesignSummary empty: expected 'no design yet', got %q", got)
	}
}

func TestProjectArtifacts_DesignSummary_Short(t *testing.T) {
	a := ProjectArtifacts{Design: "Short design"}
	if got := a.DesignSummary(); got != "Short design" {
		t.Errorf("DesignSummary short: expected 'Short design', got %q", got)
	}
}

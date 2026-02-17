package ui

import (
	"fmt"
	"testing"

	"devdeploy/internal/project"
)

// BenchmarkProjectDetailView_Render benchmarks View() rendering performance.
func BenchmarkProjectDetailView_Render(b *testing.B) {
	// Create a view with realistic data
	v := NewProjectDetailView("test-project")
	v.Resources = makeTestResources(100) // 100 resources
	v.buildItems()
	v.SetSize(80, 24)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.View()
	}
}

// BenchmarkProjectDetailView_RenderLarge benchmarks with large dataset.
func BenchmarkProjectDetailView_RenderLarge(b *testing.B) {
	v := NewProjectDetailView("test-project")
	v.Resources = makeTestResources(1000) // 1000 resources
	v.buildItems()
	v.SetSize(80, 24)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.View()
	}
}

// BenchmarkProjectDetailView_Navigation benchmarks j/k navigation.
func BenchmarkProjectDetailView_Navigation(b *testing.B) {
	v := NewProjectDetailView("test-project")
	v.Resources = makeTestResources(100)
	v.buildItems()
	v.SetSize(80, 24)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate j/k navigation
		if i%2 == 0 {
			v.Update(keyMsg("j"))
		} else {
			v.Update(keyMsg("k"))
		}
	}
}

// BenchmarkProjectDetailView_BuildItems benchmarks item building.
func BenchmarkProjectDetailView_BuildItems(b *testing.B) {
	v := NewProjectDetailView("test-project")
	v.Resources = makeTestResources(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.buildItems()
	}
}

// makeTestResources creates test resources with beads for benchmarking.
func makeTestResources(count int) []project.Resource {
	resources := make([]project.Resource, count)
	for i := 0; i < count; i++ {
		kind := project.ResourceRepo
		if i%3 == 0 {
			kind = project.ResourcePR
		}

		repoName := fmt.Sprintf("repo-%c", 'a'+i%26)
		resource := project.Resource{
			Kind:        kind,
			RepoName:    repoName,
			WorktreePath: fmt.Sprintf("/tmp/worktree-%c", 'a'+i%26),
		}

		if kind == project.ResourcePR {
			resource.PR = &project.PRInfo{
				Number:     1000 + i,
				Title:      fmt.Sprintf("Test PR %c", 'a'+i%26),
				State:      "OPEN",
				HeadRefName: fmt.Sprintf("feature-%c", 'a'+i%26),
			}
		}

		// Add 2-3 beads per resource
		beadCount := 2 + (i % 2)
		resource.Beads = make([]project.BeadInfo, beadCount)
		for j := 0; j < beadCount; j++ {
			resource.Beads[j] = project.BeadInfo{
				ID:          fmt.Sprintf("bead-%c-%d", 'a'+i%26, j),
				Title:       fmt.Sprintf("Test bead %c-%d", 'a'+i%26, j),
				Status:      "open",
				IssueType:   "task",
				Description: "This is a test bead description for benchmarking purposes.",
			}
		}

		resources[i] = resource
	}
	return resources
}

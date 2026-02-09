package rules

import (
	"strings"
	"testing"
)

func TestFiles_ReturnsExpectedRules(t *testing.T) {
	files := Files()
	if files == nil {
		t.Fatal("Files() returned nil")
	}

	expected := []string{"beads.mdc", "devdeploy.mdc"}
	for _, name := range expected {
		data, ok := files[name]
		if !ok {
			t.Errorf("missing expected file %q", name)
			continue
		}
		if len(data) == 0 {
			t.Errorf("file %q is empty", name)
		}
	}

	if len(files) != len(expected) {
		t.Errorf("expected %d files, got %d", len(expected), len(files))
	}
}

func TestFiles_BeadsMDCContent(t *testing.T) {
	files := Files()
	data := files["beads.mdc"]
	content := string(data)

	// Verify key content is present.
	checks := []string{
		"bd ready",
		"bd close",
		"bd sync",
		"alwaysApply: true",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("beads.mdc missing expected content %q", want)
		}
	}
}

func TestFiles_DevdeployMDCContent(t *testing.T) {
	files := Files()
	data := files["devdeploy.mdc"]
	content := string(data)

	// Verify key content is present.
	checks := []string{
		"dev-log/",
		"Architecture Documentation",
		"alwaysApply: true",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("devdeploy.mdc missing expected content %q", want)
		}
	}
}

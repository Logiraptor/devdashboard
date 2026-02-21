package ralph

import (
	"encoding/json"
	"fmt"
	"strings"

	"devdeploy/internal/bd"
)

// PromptData holds the variables injected into the agent prompt.
type PromptData struct {
	ID          string // Bead ID (e.g. "devdeploy-bkp.3")
	Title       string // Bead title
	Description string // Full bead description (markdown)
	IssueType   string // Issue type: "epic", "task", "bug", etc.
}

// bdShowFull mirrors the JSON shape emitted by `bd show <id> --json`,
// including the description field needed for prompt rendering.
type bdShowFull struct {
	bdShowBase
	IssueType string `json:"issue_type"`
}

// FetchPromptData runs `bd show <id> --json` and extracts the fields needed
// for prompt rendering. runBD is the command runner (pass nil for real bd).
func FetchPromptData(runBD bd.Runner, workDir string, beadID string) (*PromptData, error) {
	if runBD == nil {
		runBD = bd.Run
	}

	out, err := runBD(workDir, "show", beadID, "--json")
	if err != nil {
		return nil, fmt.Errorf("bd show %s: %w", beadID, err)
	}

	var entries []bdShowFull
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parsing bd show output: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("bd show %s returned empty result", beadID)
	}

	e := entries[0]
	return &PromptData{
		ID:          e.ID,
		Title:       e.Title,
		Description: e.Description,
		IssueType:   e.IssueType,
	}, nil
}

// Skill names for external cursor skills in ~/.cursor/skills/
const (
	SkillWorkBead   = "work-bead"
	SkillVerifyBead = "verify-bead"
)

// RenderPrompt renders a prompt that invokes the work-bead skill and provides bead context.
// The actual workflow instructions live in the external cursor skill.
func RenderPrompt(data *PromptData) (string, error) {
	return renderSkillPrompt(SkillWorkBead, data), nil
}

// RenderVerifyPrompt renders a prompt that invokes the verify-bead skill.
func RenderVerifyPrompt(data *PromptData) (string, error) {
	return renderSkillPrompt(SkillVerifyBead, data), nil
}

// renderSkillPrompt creates a prompt that invokes a skill and provides bead context.
func renderSkillPrompt(skill string, data *PromptData) string {
	var b strings.Builder

	b.WriteString("/")
	b.WriteString(skill)
	b.WriteString("\n\n")

	b.WriteString("Bead ID: ")
	b.WriteString(data.ID)
	b.WriteString("\n\n")

	b.WriteString("# ")
	b.WriteString(data.Title)
	b.WriteString("\n\n")

	if data.Description != "" {
		b.WriteString(data.Description)
	}

	return b.String()
}

package ralph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"

	"devdeploy/internal/bd"
	"devdeploy/internal/beads"
)

// PromptData holds the variables injected into the agent prompt template.
type PromptData struct {
	ID          string // Bead ID (e.g. "devdeploy-bkp.3")
	Title       string // Bead title
	Description string // Full bead description (markdown)
}

// bdShowFull mirrors the JSON shape emitted by `bd show <id> --json`,
// including the description field needed for prompt rendering.
type bdShowFull struct {
	bdShowBase
}

// FetchPromptData runs `bd show <id> --json` and extracts the fields needed
// for prompt rendering. runBD is the command runner (pass nil for real bd).
func FetchPromptData(runBD BDRunner, workDir string, beadID string) (*PromptData, error) {
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
	}, nil
}

// promptTemplate is the Go text/template used to craft the agent prompt.
// It is parsed once at init time so rendering is cheap.
var promptTemplate = template.Must(template.New("prompt").Parse(promptTemplateText))

const promptTemplateText = `You are working on bead {{.ID}}.

# {{.Title}}

{{.Description}}

---

## Workflow

1. **Claim this bead** before starting work:
   ` + "`" + `bd update {{.ID}} --status in_progress` + "`" + `

2. **Follow project conventions**: read and obey ` + "`.cursor/rules/`" + ` and ` + "`AGENTS.md`" + `.

3. **Do the work** described above. Make focused, well-tested changes.

4. **Close the bead** when the work is complete:
   ` + "`" + `bd close {{.ID}}` + "`" + `

5. **Push your work** — work is not done until pushed:
   ` + "```" + `
   git add -A && git commit -m "<concise message>"
   git pull --rebase && bd sync && git push
   ` + "```" + `

## If you need human input

If the bead is ambiguous or you need information you cannot find in the codebase:

1. Create a question bead:
   ` + "`" + `bd create "Question: <your question>" --type task --label needs-human --parent {{.ID}}` + "`" + `
2. Add a blocking dependency so the original bead drops off bd ready:
   ` + "`" + `bd dep add {{.ID}} <question-id>` + "`" + `
3. **Stop working on this bead** — do not guess. Move on.

Do NOT close this bead if you created a blocking question.`

// RenderPrompt renders the agent prompt template with the given bead data.
func RenderPrompt(data *PromptData) (string, error) {
	var buf bytes.Buffer
	if err := promptTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("rendering prompt template: %w", err)
	}
	return buf.String(), nil
}

// VerificationPromptData holds the variables injected into the verification agent prompt template.
type VerificationPromptData struct {
	ID          string         // Parent bead ID (e.g. "devdeploy-66p")
	Title       string         // Parent bead title
	Description string         // Full parent bead description (markdown)
	Children    []ChildSummary // Summary of child beads
}

// ChildSummary represents a summary of a child bead for verification.
type ChildSummary struct {
	ID      string  // Child bead ID
	Title   string  // Child bead title
	Status  string  // Child bead status (e.g. "closed", "open", "in_progress")
	Outcome Outcome // Inferred outcome based on status and labels
}

// bdShowWithDependents mirrors the JSON shape emitted by `bd show <id> --json`,
// including dependents needed for verification prompt rendering.
type bdShowWithDependents struct {
	bdShowBase
	Dependents []bdShowDependent `json:"dependents"`
}

// bdShowDependent represents a dependent/child bead in bd show --json output.
type bdShowDependent struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Status string   `json:"status"`
	Labels []string `json:"labels"`
}

// FetchVerificationPromptData runs `bd show <id> --json` and extracts the parent bead info
// plus child beads summary. runBD is the command runner (pass nil for real bd).
func FetchVerificationPromptData(runBD BDRunner, workDir string, beadID string) (*VerificationPromptData, error) {
	if runBD == nil {
		runBD = bd.Run
	}

	out, err := runBD(workDir, "show", beadID, "--json")
	if err != nil {
		return nil, fmt.Errorf("bd show %s: %w", beadID, err)
	}

	var entries []bdShowWithDependents
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parsing bd show output: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("bd show %s returned empty result", beadID)
	}

	e := entries[0]
	children := make([]ChildSummary, 0, len(e.Dependents))
	for _, dep := range e.Dependents {
		outcome := inferOutcome(dep.Status, dep.Labels)
		children = append(children, ChildSummary{
			ID:      dep.ID,
			Title:   dep.Title,
			Status:  dep.Status,
			Outcome: outcome,
		})
	}

	return &VerificationPromptData{
		ID:          e.ID,
		Title:       e.Title,
		Description: e.Description,
		Children:    children,
	}, nil
}

// inferOutcome determines the Outcome for a child bead based on its status and labels.
func inferOutcome(status string, labels []string) Outcome {
	if status == beads.StatusClosed {
		return OutcomeSuccess
	}
	// Check for needs-human label to identify questions
	for _, label := range labels {
		if label == beads.LabelNeedsHuman {
			return OutcomeQuestion
		}
	}
	// Open or in_progress without needs-human = failure
	return OutcomeFailure
}

// verificationPromptTemplate is the Go text/template used to craft the verification agent prompt.
// It is parsed once at init time so rendering is cheap.
var verificationPromptTemplate = template.Must(template.New("verificationPrompt").Parse(verificationPromptTemplateText))

const verificationPromptTemplateText = `You are verifying completion of epic bead {{.ID}}.

# {{.Title}}

{{.Description}}

---

## Children Summary

{{- if .Children}}
{{range .Children}}
- **{{.ID}}**: {{.Title}}
  - Status: {{.Status}}
  - Outcome: {{.Outcome}}
{{- end}}
{{- else}}
No child beads found.
{{- end}}

---

## Verification Workflow

1. **Review completed work**: Examine what each child bead accomplished. Check code changes, tests, and documentation.

2. **Unblock failures**: For any children with "failure" outcome:
   - Investigate why they failed
   - Fix issues directly if straightforward
   - Create new child beads for complex fixes that need separate work
   - Update status: ` + "`" + `bd update <child-id> --status in_progress` + "`" + ` or create new beads as needed

3. **Handle questions**: For children with "question" outcome:
   - Review the question beads (they have ` + "`" + `needs-human` + "`" + ` label)
   - If you can answer them, do so and close the question beads
   - If they need human input, leave them open

4. **Run quality checks**: Before closing the epic, ensure:
   - All tests pass: run test suite
   - Code is linted: run linters
   - No obvious issues remain

5. **Close the epic** when satisfied:
   ` + "`" + `bd close {{.ID}}` + "`" + `

6. **Push your work** — work is not done until pushed:
   ` + "```" + `
   git add -A && git commit -m "<concise message>"
   git pull --rebase && bd sync && git push
   ` + "```" + `

## Important Notes

- You are running in the main repository (no worktree) since children's work should already be merged
- Focus on verification, unblocking, and quality assurance
- Only close the epic when all children are successfully completed and quality checks pass`

// RenderVerificationPrompt renders the verification agent prompt template with the given verification data.
func RenderVerificationPrompt(data *VerificationPromptData) (string, error) {
	var buf bytes.Buffer
	if err := verificationPromptTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("rendering verification prompt template: %w", err)
	}
	return buf.String(), nil
}

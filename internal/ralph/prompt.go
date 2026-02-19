package ralph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"

	"devdeploy/internal/bd"
)

// PromptData holds the variables injected into the agent prompt template.
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

// promptTemplate is the Go text/template used to craft the agent prompt for regular beads.
// It is parsed once at init time so rendering is cheap.
var promptTemplate = template.Must(template.New("prompt").Parse(promptTemplateText))

// epicPromptTemplate is the Go text/template used to craft the agent prompt for epics.
// It includes instructions for processing children sequentially.
var epicPromptTemplate = template.Must(template.New("epicPrompt").Parse(epicPromptTemplateText))

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

## If you discover additional work

Do NOT get sidetracked by unrelated issues you discover. Stay focused on THIS bead.

- Create child beads for off-topic work: ` + "`" + `bd create "..." --parent {{.ID}}` + "`" + `
- For parallelizable subtasks, spawn subagents to work on them
- Only close THIS bead when its specific work is complete

## If you need human input

If the bead is ambiguous or you need information you cannot find in the codebase:

1. Create a question bead:
   ` + "`" + `bd create "Question: <your question>" --type task --label needs-human --parent {{.ID}}` + "`" + `
2. Add a blocking dependency so the original bead drops off bd ready:
   ` + "`" + `bd dep add {{.ID}} <question-id>` + "`" + `
3. **Stop working on this bead** — do not guess. Move on.

Do NOT close this bead if you created a blocking question.`

const epicPromptTemplateText = `You are working on epic {{.ID}}.

# {{.Title}}

{{.Description}}

---

## Workflow

1. **Claim this epic** before starting work:
   ` + "`" + `bd update {{.ID}} --status in_progress` + "`" + `

2. **Follow project conventions**: read and obey ` + "`.cursor/rules/`" + ` and ` + "`AGENTS.md`" + `.

3. **Process children sequentially**: This epic has child issues that must be completed before closing the epic.
   - Use ` + "`" + `bd ready --parent {{.ID}}` + "`" + ` to find available children
   - For each child:
     a. Claim it: ` + "`" + `bd update <child-id> --status in_progress` + "`" + `
     b. Implement the work described in that child
     c. Close it when complete: ` + "`" + `bd close <child-id>` + "`" + `
   - **CRITICAL**: You MUST close each child as you complete its work
   - **CRITICAL**: Only close the epic when ALL children are closed

4. **Close the epic** ONLY when all children are closed:
   ` + "`" + `bd close {{.ID}}` + "`" + `
   
   **Do NOT close the epic if any children remain open.** Check with ` + "`" + `bd ready --parent {{.ID}}` + "`" + ` first.

5. **Push your work** — work is not done until pushed:
   ` + "```" + `
   git add -A && git commit -m "<concise message>"
   git pull --rebase && bd sync && git push
   ` + "```" + `

## Epic Completion Rules

- **Never close an epic with open children** — this leaves work orphaned
- Process children one at a time, closing each as you complete it
- If you cannot complete all children in this session, leave the epic open (status: in_progress)
- Only close the epic when ` + "`" + `bd ready --parent {{.ID}}` + "`" + ` returns no results

## If you discover additional work

Do NOT get sidetracked by unrelated issues you discover. Stay focused on THIS epic and its children.

- Create child beads for off-topic work: ` + "`" + `bd create "..." --parent {{.ID}}` + "`" + `
- For parallelizable subtasks, spawn subagents to work on them
- Only close THIS epic when all its children are complete

## If you need human input

If the epic or a child is ambiguous or you need information you cannot find in the codebase:

1. Create a question bead:
   ` + "`" + `bd create "Question: <your question>" --type task --label needs-human --parent {{.ID}}` + "`" + `
2. Add a blocking dependency so the original bead drops off bd ready:
   ` + "`" + `bd dep add <blocked-bead-id> <question-id>` + "`" + `
3. **Stop working on this epic** — do not guess. Move on.

Do NOT close this epic if you created a blocking question.`

// RenderPrompt renders the agent prompt template with the given bead data.
// It automatically selects the epic template if IssueType is "epic".
func RenderPrompt(data *PromptData) (string, error) {
	var buf bytes.Buffer
	var tmpl *template.Template
	
	if data.IssueType == "epic" {
		tmpl = epicPromptTemplate
	} else {
		tmpl = promptTemplate
	}
	
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("rendering prompt template: %w", err)
	}
	return buf.String(), nil
}

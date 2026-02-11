package ralph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"
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
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// FetchPromptData runs `bd show <id> --json` and extracts the fields needed
// for prompt rendering. runBD is the command runner (pass nil for real bd).
func FetchPromptData(runBD BDRunner, workDir string, beadID string) (*PromptData, error) {
	if runBD == nil {
		runBD = runBDReal
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

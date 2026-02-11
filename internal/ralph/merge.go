package ralph

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

// MergeBranches merges sourceBranch into targetBranch in the given repository.
// If the repository is a worktree, it merges within that worktree.
// If hooks should be disabled, set disableHooks to true.
// Returns an error if the merge fails.
func MergeBranches(repoPath, targetBranch, sourceBranch string, disableHooks bool) error {
	// Verify the repository exists
	if _, err := os.Stat(repoPath); err != nil {
		return fmt.Errorf("repository path %s: %w", repoPath, err)
	}

	// Verify target branch exists
	if err := exec.Command("git", "-C", repoPath, "rev-parse", "--verify", targetBranch).Run(); err != nil {
		return fmt.Errorf("target branch %s does not exist: %w", targetBranch, err)
	}

	// Verify source branch exists (try local first, then remote)
	sourceRef := sourceBranch
	if err := exec.Command("git", "-C", repoPath, "rev-parse", "--verify", sourceRef).Run(); err != nil {
		sourceRef = "origin/" + sourceBranch
		if err := exec.Command("git", "-C", repoPath, "rev-parse", "--verify", sourceRef).Run(); err != nil {
			return fmt.Errorf("source branch %s not found locally or on origin: %w", sourceBranch, err)
		}
	}

	// Get current branch to restore later
	currentBranchCmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	currentBranchOut, err := currentBranchCmd.Output()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}
	currentBranch := strings.TrimSpace(string(currentBranchOut))

	// Checkout target branch
	checkoutCmd := exec.Command("git", "-C", repoPath, "checkout", targetBranch)
	var checkoutStderr bytes.Buffer
	checkoutCmd.Stderr = &checkoutStderr
	if err := checkoutCmd.Run(); err != nil {
		msg := strings.TrimSpace(checkoutStderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("checkout %s: %s", targetBranch, msg)
	}

	// Restore original branch on return (best effort)
	defer func() {
		_ = exec.Command("git", "-C", repoPath, "checkout", currentBranch).Run()
	}()

	// Prepare git command args
	var gitArgs []string
	if disableHooks {
		// Create empty hooks directory to disable hooks
		emptyHooksDir, err := os.MkdirTemp("", "devdeploy-nohooks")
		if err != nil {
			return fmt.Errorf("create temp hooks dir: %w", err)
		}
		defer func() { _ = os.RemoveAll(emptyHooksDir) }()
		gitArgs = []string{"-C", repoPath, "-c", "core.hooksPath=" + emptyHooksDir}
	} else {
		gitArgs = []string{"-C", repoPath}
	}

	// Perform merge
	mergeArgs := append(gitArgs, "merge", sourceRef, "--no-edit")
	mergeCmd := exec.Command("git", mergeArgs...)
	var mergeStderr bytes.Buffer
	mergeCmd.Stderr = &mergeStderr
	if err := mergeCmd.Run(); err != nil {
		msg := strings.TrimSpace(mergeStderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("merge %s into %s: %s", sourceRef, targetBranch, msg)
	}

	return nil
}

// MergePromptData holds the variables injected into the merge agent prompt template.
type MergePromptData struct {
	ID            string // Bead ID (e.g. "devdeploy-x99.3")
	Title         string // Bead title
	Description   string // Full bead description (markdown)
	TargetBranch  string // Target branch to merge into
	SourceBranch  string // Source branch to merge from
	RepoPath      string // Repository path
}

// mergePromptTemplate is the Go text/template used to craft the merge agent prompt.
// It is parsed once at init time so rendering is cheap.
var mergePromptTemplate = template.Must(template.New("mergePrompt").Parse(mergePromptTemplateText))

const mergePromptTemplateText = `You are working on bead {{.ID}}.

# {{.Title}}

{{.Description}}

---

## Merge Task

Merge branch **{{.SourceBranch}}** into **{{.TargetBranch}}** in repository: ` + "`{{.RepoPath}}`" + `

## Workflow

1. **Claim this bead** before starting work:
   ` + "`" + `bd update {{.ID}} --status in_progress` + "`" + `

2. **Follow project conventions**: read and obey ` + "`.cursor/rules/`" + ` and ` + "`AGENTS.md`" + `.

3. **Perform the merge**:
   - Ensure you're on the target branch (` + "`{{.TargetBranch}}`" + `)
   - Merge ` + "`{{.SourceBranch}}`" + ` into ` + "`{{.TargetBranch}}`" + `
   - Resolve any conflicts if they occur
   - Test that the merge was successful

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

// RenderMergePrompt renders the merge agent prompt template with the given merge data.
func RenderMergePrompt(data *MergePromptData) (string, error) {
	var buf bytes.Buffer
	if err := mergePromptTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("rendering merge prompt template: %w", err)
	}
	return buf.String(), nil
}

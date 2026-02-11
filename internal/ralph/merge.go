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
// If beadID is provided and conflicts occur, a question bead will be created automatically.
// Returns an error if the merge fails.
func MergeBranches(repoPath, targetBranch, sourceBranch string, disableHooks bool, beadID string) error {
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
		
		// Check if this is a merge conflict
		if hasMergeConflicts(repoPath) {
			// If beadID is provided, create a question bead for the conflict
			if beadID != "" {
				if createErr := createQuestionBeadForMergeConflict(repoPath, beadID, targetBranch, sourceBranch); createErr != nil {
					// Log the error but don't fail the merge error - the conflict is the real issue
					_ = createErr // Best effort - if bead creation fails, still return merge error
				}
			}
			return fmt.Errorf("merge %s into %s: conflicts detected: %s", sourceRef, targetBranch, msg)
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

// hasMergeConflicts checks if there are merge conflicts in the repository.
func hasMergeConflicts(repoPath string) bool {
	// Check git status for conflict markers
	statusCmd := exec.Command("git", "-C", repoPath, "status", "--porcelain")
	statusOut, err := statusCmd.Output()
	if err != nil {
		return false
	}
	
	// Look for conflict markers in status output (UU = both modified, AA = both added, etc.)
	statusStr := string(statusOut)
	conflictMarkers := []string{"UU ", "AA ", "DD ", "AU ", "UA ", "DU ", "UD "}
	for _, marker := range conflictMarkers {
		if strings.Contains(statusStr, marker) {
			return true
		}
	}
	
	// Also check for conflict markers in files directly
	lsFilesCmd := exec.Command("git", "-C", repoPath, "ls-files", "-u")
	lsFilesOut, err := lsFilesCmd.Output()
	if err == nil && len(lsFilesOut) > 0 {
		return true
	}
	
	return false
}

// createQuestionBeadForMergeConflict creates a question bead when merge conflicts cannot be resolved.
func createQuestionBeadForMergeConflict(repoPath, beadID, targetBranch, sourceBranch string) error {
	// Get conflict details for the description
	statusCmd := exec.Command("git", "-C", repoPath, "status", "--short")
	statusOut, err := statusCmd.Output()
	conflictDetails := ""
	if err == nil {
		conflictDetails = strings.TrimSpace(string(statusOut))
	}
	
	// Build description with context
	description := fmt.Sprintf(`Merge conflicts detected when attempting to merge **%s** into **%s**.

## Context
The merge agent attempted to merge these branches but encountered conflicts that require human intervention.

## Conflict Details
%s

## Next Steps
Please resolve the conflicts manually and complete the merge.`, sourceBranch, targetBranch, conflictDetails)
	
	// Create the question bead
	title := fmt.Sprintf("Question: Merge conflicts in %s → %s", sourceBranch, targetBranch)
	createArgs := []string{
		"create",
		"--title", title,
		"--description", description,
		"--type", "task",
		"--label", "needs-human",
	}
	if beadID != "" {
		createArgs = append(createArgs, "--parent", beadID)
	}
	
	createCmd := exec.Command("bd", createArgs...)
	createCmd.Dir = repoPath
	var createStderr bytes.Buffer
	createCmd.Stderr = &createStderr
	createOut, err := createCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to create question bead: %s: %w", strings.TrimSpace(createStderr.String()), err)
	}
	
	// Extract the new bead ID from output (bd create outputs the ID)
	questionBeadID := strings.TrimSpace(string(createOut))
	if questionBeadID == "" {
		return fmt.Errorf("bd create did not return a bead ID")
	}
	
	// Add blocking dependency: original bead depends on question bead
	depArgs := []string{"dep", "add", beadID, questionBeadID}
	depCmd := exec.Command("bd", depArgs...)
	depCmd.Dir = repoPath
	var depStderr bytes.Buffer
	depCmd.Stderr = &depStderr
	if err := depCmd.Run(); err != nil {
		return fmt.Errorf("failed to add blocking dependency: %s: %w", strings.TrimSpace(depStderr.String()), err)
	}
	
	return nil
}

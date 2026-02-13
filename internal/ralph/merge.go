package ralph

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"devdeploy/internal/beads"
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
		if msg != "" {
			return fmt.Errorf("checkout %s: %s: %w", targetBranch, msg, err)
		}
		return fmt.Errorf("checkout %s: %w", targetBranch, err)
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
		
		// Check if this is a merge conflict
		if hasMergeConflicts(repoPath) {
			// If beadID is provided, create a question bead for the conflict
			if beadID != "" {
				if createErr := createQuestionBeadForMergeConflict(repoPath, beadID, targetBranch, sourceBranch); createErr != nil {
					// Log the error but don't fail the merge error - the conflict is the real issue
					_ = createErr // Best effort - if bead creation fails, still return merge error
				}
			}
			// Abort the merge to leave repo in a clean state (important for concurrent execution)
			abortCmd := exec.Command("git", "-C", repoPath, "merge", "--abort")
			_ = abortCmd.Run() // Best effort - if abort fails, we still return the conflict error
			
			if msg != "" {
				return fmt.Errorf("merge %s into %s: conflicts detected: %s: %w", sourceRef, targetBranch, msg, err)
			}
			return fmt.Errorf("merge %s into %s: conflicts detected: %w", sourceRef, targetBranch, err)
		}
		
		if msg != "" {
			return fmt.Errorf("merge %s into %s: %s: %w", sourceRef, targetBranch, msg, err)
		}
		return fmt.Errorf("merge %s into %s: %w", sourceRef, targetBranch, err)
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

// conflictResolutionPromptTemplate is a simplified prompt for resolving merge conflicts.
var conflictResolutionPromptTemplate = template.Must(template.New("conflictResolution").Parse(conflictResolutionPromptText))

const conflictResolutionPromptText = `# Merge Conflict Resolution

You need to resolve merge conflicts between **{{.SourceBranch}}** and **{{.TargetBranch}}**.

## Current State

The repository is in a conflicted state after attempting to merge ` + "`{{.SourceBranch}}`" + ` into ` + "`{{.TargetBranch}}`" + `.

Working directory: ` + "`{{.RepoPath}}`" + `

## Conflicting Files

{{.ConflictDetails}}

## Your Task

1. **Examine the conflicts** - Look at each conflicting file to understand what changed on both branches
2. **Resolve conflicts** - Edit the files to combine changes appropriately, removing conflict markers (<<<<<<, ======, >>>>>>)
3. **Stage resolved files** - ` + "`git add <resolved-file>`" + `
4. **Complete the merge** - ` + "`git commit --no-edit`" + ` (uses the default merge commit message)
5. **Verify** - Run ` + "`git status`" + ` to confirm the merge is complete

## Important

- Do NOT abort the merge
- Do NOT create new branches
- Do NOT push (the caller will handle that)
- If conflicts are too complex to resolve confidently, you may leave the merge incomplete and explain why

## Context from Original Work

Bead: {{.BeadID}}
{{if .BeadTitle}}Title: {{.BeadTitle}}{{end}}
`

// ConflictResolutionData holds data for the conflict resolution prompt.
type ConflictResolutionData struct {
	TargetBranch    string
	SourceBranch    string
	RepoPath        string
	ConflictDetails string
	BeadID          string
	BeadTitle       string
}

// RenderConflictResolutionPrompt renders the conflict resolution prompt.
func RenderConflictResolutionPrompt(data *ConflictResolutionData) (string, error) {
	var buf bytes.Buffer
	if err := conflictResolutionPromptTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("rendering conflict resolution prompt: %w", err)
	}
	return buf.String(), nil
}

// getConflictDetails returns a string describing the current merge conflicts.
func getConflictDetails(repoPath string) string {
	statusCmd := exec.Command("git", "-C", repoPath, "status", "--short")
	statusOut, err := statusCmd.Output()
	if err != nil {
		return "(unable to get conflict details)"
	}
	return strings.TrimSpace(string(statusOut))
}

// MergeWithAgentResolution attempts to merge sourceBranch into targetBranch.
// If conflicts occur, it spawns an agent to resolve them automatically.
// If the agent fails to resolve conflicts, it creates a question bead and aborts.
// Returns nil on success, error on failure.
func MergeWithAgentResolution(ctx context.Context, repoPath, targetBranch, sourceBranch string, beadID, beadTitle string, agentTimeout time.Duration) error {
	// First, try the merge
	err := MergeBranches(repoPath, targetBranch, sourceBranch, true, "")
	if err == nil {
		return nil // Merge succeeded without conflicts
	}

	// Check if error was due to conflicts (MergeBranches aborts on conflict)
	if !strings.Contains(err.Error(), "conflicts detected") {
		return err // Not a conflict error, return as-is
	}

	// Conflicts detected but were aborted. Re-attempt the merge to get into conflicted state
	// for agent resolution.
	checkoutCmd := exec.Command("git", "-C", repoPath, "checkout", targetBranch)
	if checkoutErr := checkoutCmd.Run(); checkoutErr != nil {
		return fmt.Errorf("checkout %s for conflict resolution: %w", targetBranch, checkoutErr)
	}

	mergeCmd := exec.Command("git", "-C", repoPath, "merge", sourceBranch, "--no-edit")
	mergeCmd.Run() // This will fail with conflicts, which is expected

	if !hasMergeConflicts(repoPath) {
		// Somehow no conflicts now - merge must have succeeded
		return nil
	}

	// Get conflict details for the prompt
	conflictDetails := getConflictDetails(repoPath)

	// Render conflict resolution prompt
	promptData := &ConflictResolutionData{
		TargetBranch:    targetBranch,
		SourceBranch:    sourceBranch,
		RepoPath:        repoPath,
		ConflictDetails: conflictDetails,
		BeadID:          beadID,
		BeadTitle:       beadTitle,
	}
	prompt, renderErr := RenderConflictResolutionPrompt(promptData)
	if renderErr != nil {
		// Abort merge and return error
		abortCmd := exec.Command("git", "-C", repoPath, "merge", "--abort")
		_ = abortCmd.Run()
		return fmt.Errorf("rendering conflict resolution prompt: %w", renderErr)
	}

	// Run agent to resolve conflicts
	var opts []Option
	if agentTimeout > 0 {
		opts = append(opts, WithTimeout(agentTimeout))
	}
	result, agentErr := RunAgent(ctx, repoPath, prompt, opts...)
	if agentErr != nil {
		// Agent failed to run - abort merge and create question bead
		abortCmd := exec.Command("git", "-C", repoPath, "merge", "--abort")
		_ = abortCmd.Run()
		if beadID != "" {
			_ = createQuestionBeadForMergeConflict(repoPath, beadID, targetBranch, sourceBranch)
		}
		return fmt.Errorf("merge conflict resolution agent failed: %w", agentErr)
	}

	// Check if agent resolved the conflicts
	if hasMergeConflicts(repoPath) {
		// Agent didn't resolve all conflicts - abort and create question bead
		abortCmd := exec.Command("git", "-C", repoPath, "merge", "--abort")
		_ = abortCmd.Run()
		if beadID != "" {
			_ = createQuestionBeadForMergeConflict(repoPath, beadID, targetBranch, sourceBranch)
		}
		return fmt.Errorf("agent could not resolve merge conflicts (exit code: %d)", result.ExitCode)
	}

	// Conflicts resolved! Verify the merge commit exists
	statusCmd := exec.Command("git", "-C", repoPath, "status", "--porcelain")
	statusOut, _ := statusCmd.Output()
	if len(strings.TrimSpace(string(statusOut))) > 0 {
		// There are uncommitted changes - agent may have resolved but not committed
		// Try to complete the merge commit
		commitCmd := exec.Command("git", "-C", repoPath, "commit", "--no-edit")
		if commitErr := commitCmd.Run(); commitErr != nil {
			abortCmd := exec.Command("git", "-C", repoPath, "merge", "--abort")
			_ = abortCmd.Run()
			if beadID != "" {
				_ = createQuestionBeadForMergeConflict(repoPath, beadID, targetBranch, sourceBranch)
			}
			return fmt.Errorf("agent resolved conflicts but failed to commit: %w", commitErr)
		}
	}

	return nil
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
		"--label", beads.LabelNeedsHuman,
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

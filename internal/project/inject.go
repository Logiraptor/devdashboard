package project

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"devdeploy/internal/rules"
)

// excludeEntries are the paths added to .git/info/exclude so injected
// files are invisible to git status, diff, etc.
var excludeEntries = []string{".cursor/"}

// InjectWorktreeRules writes Cursor rule files and a dev-log directory
// into a worktree, then adds them to the repo's git exclude file
// so they are invisible to git.
//
// The operation is idempotent: existing files with matching content are
// left untouched, and duplicate exclude entries are not added.
func InjectWorktreeRules(worktreePath string) error {
	// 1. Create .cursor/rules/ and write rule files.
	rulesDir := filepath.Join(worktreePath, ".cursor", "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		return fmt.Errorf("create rules dir: %w", err)
	}

	for name, content := range rules.Files() {
		dst := filepath.Join(rulesDir, name)
		// Skip if file already exists with identical content.
		if existing, err := os.ReadFile(dst); err == nil && bytes.Equal(existing, content) {
			continue
		}
		if err := os.WriteFile(dst, content, 0644); err != nil {
			return fmt.Errorf("write rule %s: %w", name, err)
		}
	}

	// 2. Create dev-log/ directory.
	devLogDir := filepath.Join(worktreePath, "dev-log")
	if err := os.MkdirAll(devLogDir, 0755); err != nil {
		return fmt.Errorf("create dev-log dir: %w", err)
	}

	// 3. Add entries to the common git exclude file.
	// Git reads info/exclude from the common dir, NOT the per-worktree
	// gitdir. For regular repos the common dir IS .git/; for worktrees
	// it's the main repo's .git/ (found via the "commondir" file).
	commonDir, err := resolveGitCommonDir(worktreePath)
	if err != nil {
		return fmt.Errorf("resolve git common dir: %w", err)
	}
	if err := ensureExcludeEntries(commonDir, excludeEntries); err != nil {
		return fmt.Errorf("update exclude: %w", err)
	}
	return nil
}

// resolveGitCommonDir returns the git common directory for a worktree.
// Git reads info/exclude from the common dir, not the per-worktree gitdir.
//
// For a regular repo (.git is a directory), the common dir is .git/ itself.
// For a worktree (.git is a file with "gitdir: <path>"), the per-worktree
// gitdir contains a "commondir" file with a relative path to the shared
// git directory.
func resolveGitCommonDir(worktreePath string) (string, error) {
	dotGit := filepath.Join(worktreePath, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		return "", err
	}
	// Regular repo: .git is a directory and is itself the common dir.
	if info.IsDir() {
		return dotGit, nil
	}

	// Worktree: .git is a file with "gitdir: <path>"
	data, err := os.ReadFile(dotGit)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir: ") {
		return "", fmt.Errorf(".git file has unexpected format: %s", line)
	}
	gitDir := strings.TrimPrefix(line, "gitdir: ")
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(worktreePath, gitDir)
	}
	gitDir = filepath.Clean(gitDir)

	// Read the "commondir" file to find the shared git directory.
	// This file contains a relative path (typically "../..") from
	// the worktree gitdir to the main repo's .git/.
	commonDirFile := filepath.Join(gitDir, "commondir")
	cdData, err := os.ReadFile(commonDirFile)
	if err != nil {
		// No commondir file — fall back to the gitdir itself.
		// This shouldn't happen for real worktrees but is safe.
		return gitDir, nil
	}
	commonRel := strings.TrimSpace(string(cdData))
	if !filepath.IsAbs(commonRel) {
		commonRel = filepath.Join(gitDir, commonRel)
	}
	return filepath.Clean(commonRel), nil
}

// ensureExcludeEntries appends entries to <gitDir>/info/exclude, skipping
// any that are already present.
func ensureExcludeEntries(gitDir string, entries []string) (err error) {
	infoDir := filepath.Join(gitDir, "info")
	if err := os.MkdirAll(infoDir, 0755); err != nil {
		return err
	}

	excludePath := filepath.Join(infoDir, "exclude")
	existing, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Build set of existing lines for dedup.
	lines := make(map[string]bool)
	for _, line := range strings.Split(string(existing), "\n") {
		lines[strings.TrimSpace(line)] = true
	}

	var toAdd []string
	for _, entry := range entries {
		if !lines[entry] {
			toAdd = append(toAdd, entry)
		}
	}
	if len(toAdd) == 0 {
		return nil
	}

	// Ensure existing content ends with a newline before appending.
	prefix := ""
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		prefix = "\n"
	}

	f, err := os.OpenFile(excludePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	_, err = f.WriteString(prefix + strings.Join(toAdd, "\n") + "\n")
	return err
}

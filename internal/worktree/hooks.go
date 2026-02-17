package worktree

import (
	"fmt"
	"os"
)

// DisableHooksConfig returns git config arguments that disable hooks
// by setting core.hooksPath to an empty temporary directory.
// The caller is responsible for cleaning up the temporary directory
// returned in cleanup.
func DisableHooksConfig() (config []string, cleanup func() error, err error) {
	emptyHooksDir, err := os.MkdirTemp("", "devdeploy-nohooks")
	if err != nil {
		return nil, nil, fmt.Errorf("create temp dir: %w", err)
	}
	config = []string{"-c", "core.hooksPath=" + emptyHooksDir}
	cleanup = func() error {
		return os.RemoveAll(emptyHooksDir)
	}
	return config, cleanup, nil
}

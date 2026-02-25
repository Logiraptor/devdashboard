package session

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResourceKey_String(t *testing.T) {
	tests := []struct {
		name string
		rk   ResourceKey
		want string
	}{
		{"repo key", NewRepoKey("devdeploy"), "repo:devdeploy"},
		{"PR key", NewPRKey("devdeploy", 42), "pr:devdeploy:#42"},
		{"PR key single digit", NewPRKey("grafana", 7), "pr:grafana:#7"},
		{"PR key zero still renders as PR", NewPRKey("grafana", 0), "pr:grafana:#0"},
		{"worktree key", NewWorktreeKey("devdeploy", "feature/x"), "worktree:devdeploy:feature/x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.rk.String())
		})
	}
}

func TestResourceKey_Kind(t *testing.T) {
	tests := []struct {
		name string
		rk   ResourceKey
		want string
	}{
		{"repo", NewRepoKey("devdeploy"), "repo"},
		{"PR", NewPRKey("devdeploy", 42), "pr"},
		{"worktree", NewWorktreeKey("devdeploy", "feature/x"), "worktree"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.rk.Kind())
		})
	}
}

func TestResourceKey_RepoName(t *testing.T) {
	tests := []struct {
		name     string
		rk       ResourceKey
		wantRepo string
	}{
		{"repo", NewRepoKey("devdeploy"), "devdeploy"},
		{"PR", NewPRKey("grafana", 7), "grafana"},
		{"worktree", NewWorktreeKey("grafana", "feature/x"), "grafana"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantRepo, tt.rk.RepoName())
		})
	}
}

func TestResourceKey_PRNumber(t *testing.T) {
	tests := []struct {
		name   string
		rk     ResourceKey
		wantPR int
	}{
		{"repo (should be 0)", NewRepoKey("devdeploy"), 0},
		{"PR", NewPRKey("devdeploy", 42), 42},
		{"PR single digit", NewPRKey("grafana", 7), 7},
		{"worktree (should be 0)", NewWorktreeKey("devdeploy", "feature/x"), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantPR, tt.rk.PRNumber())
		})
	}
}

func TestResourceKey_IsValid(t *testing.T) {
	tests := []struct {
		name string
		rk   ResourceKey
		want bool
	}{
		{"valid repo", NewRepoKey("devdeploy"), true},
		{"valid PR", NewPRKey("devdeploy", 42), true},
		{"valid worktree", NewWorktreeKey("devdeploy", "feature/x"), true},
		{"empty repo name", ResourceKey{kind: "repo", repoName: ""}, false},
		{"empty PR repo name", ResourceKey{kind: "pr", repoName: "", prNumber: 42}, false},
		{"PR with zero number", ResourceKey{kind: "pr", repoName: "devdeploy", prNumber: 0}, false},
		{"PR with negative number", ResourceKey{kind: "pr", repoName: "devdeploy", prNumber: -1}, false},
		{"worktree with empty branch", ResourceKey{kind: "worktree", repoName: "devdeploy", branch: ""}, false},
		{"invalid kind", ResourceKey{kind: "invalid", repoName: "devdeploy"}, false},
		{"unknown kind with valid repo", ResourceKey{kind: "unknown", repoName: "devdeploy", prNumber: 0}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.rk.IsValid(), "key: %+v", tt.rk)
		})
	}
}

func TestParseResourceKey(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    ResourceKey
		wantErr bool
		errMsg  string
	}{
		// Valid cases
		{"valid repo", "repo:devdeploy", NewRepoKey("devdeploy"), false, ""},
		{"valid repo with dash", "repo:my-repo", NewRepoKey("my-repo"), false, ""},
		{"valid repo with underscore", "repo:my_repo", NewRepoKey("my_repo"), false, ""},
		{"valid PR", "pr:devdeploy:#42", NewPRKey("devdeploy", 42), false, ""},
		{"valid PR single digit", "pr:grafana:#7", NewPRKey("grafana", 7), false, ""},
		{"valid PR large number", "pr:repo:#9999", NewPRKey("repo", 9999), false, ""},
		{"valid worktree", "worktree:devdeploy:feature/x", NewWorktreeKey("devdeploy", "feature/x"), false, ""},
		{"valid worktree with colon in branch", "worktree:devdeploy:fix:thing", NewWorktreeKey("devdeploy", "fix:thing"), false, ""},
		{"repo with whitespace trimmed", "  repo:devdeploy  ", NewRepoKey("devdeploy"), false, ""},
		{"PR with whitespace trimmed", "  pr:devdeploy:#42  ", NewPRKey("devdeploy", 42), false, ""},
		{"worktree with whitespace trimmed", "  worktree:devdeploy:feature/x  ", NewWorktreeKey("devdeploy", "feature/x"), false, ""},

		// Invalid cases
		{"empty string", "", ResourceKey{}, true, "invalid resource key format"},
		{"missing colon", "repodevdeploy", ResourceKey{}, true, "invalid resource key format"},
		{"invalid kind", "invalid:devdeploy", ResourceKey{}, true, "invalid resource key kind"},
		{"worktree with too few parts", "worktree:devdeploy", ResourceKey{}, true, "invalid worktree key format"},
		{"worktree with empty branch", "worktree:devdeploy:", ResourceKey{}, true, "worktree branch cannot be empty"},
		{"repo with too many parts", "repo:devdeploy:extra", ResourceKey{}, true, "invalid repo key format"},
		{"PR with too few parts", "pr:devdeploy", ResourceKey{}, true, "invalid PR key format"},
		{"PR with too many parts", "pr:devdeploy:#42:extra", ResourceKey{}, true, "invalid PR key format"},
		{"PR missing # prefix", "pr:devdeploy:42", ResourceKey{}, true, "invalid PR number format"},
		{"PR with invalid number", "pr:devdeploy:#abc", ResourceKey{}, true, "invalid PR number"},
		{"PR with zero number", "pr:devdeploy:#0", ResourceKey{}, true, "PR number must be positive"},
		{"PR with negative number", "pr:devdeploy:#-1", ResourceKey{}, true, "PR number must be positive"},
		{"empty repo name", "repo:", ResourceKey{}, true, "repo name cannot be empty"},
		{"empty PR repo name", "pr::#42", ResourceKey{}, true, "repo name cannot be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseResourceKey(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					require.ErrorContains(t, err, tt.errMsg)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want.String(), got.String())
			require.Equal(t, tt.want.Kind(), got.Kind())
			require.Equal(t, tt.want.RepoName(), got.RepoName())
			require.Equal(t, tt.want.PRNumber(), got.PRNumber())
		})
	}
}

func TestParseResourceKey_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		rk   ResourceKey
	}{
		{"repo", NewRepoKey("devdeploy")},
		{"PR", NewPRKey("devdeploy", 42)},
		{"PR single digit", NewPRKey("grafana", 7)},
		{"worktree", NewWorktreeKey("devdeploy", "feature/x")},
		{"worktree with colon", NewWorktreeKey("devdeploy", "fix:thing")},
		{"repo with dash", NewRepoKey("my-repo")},
		{"PR with dash repo", NewPRKey("my-repo", 123)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert to string and back
			s := tt.rk.String()
			parsed, err := ParseResourceKey(s)
			require.NoError(t, err)
			require.Equal(t, tt.rk.String(), parsed.String())
			require.Equal(t, tt.rk.Kind(), parsed.Kind())
			require.Equal(t, tt.rk.RepoName(), parsed.RepoName())
			require.Equal(t, tt.rk.PRNumber(), parsed.PRNumber())
		})
	}
}

func TestResourceKey_SessionName(t *testing.T) {
	tests := []struct {
		name string
		rk   ResourceKey
		want string
	}{
		{"repo", NewRepoKey("devdeploy"), "dd-repo-devdeploy"},
		{"pr", NewPRKey("devdeploy", 42), "dd-pr-devdeploy-42"},
		{"worktree slash branch", NewWorktreeKey("devdeploy", "feature/x"), "dd-worktree-devdeploy-feature-x"},
		{"collapses dashes", NewWorktreeKey("devdeploy", "feature//x"), "dd-worktree-devdeploy-feature-x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.rk.SessionName())
		})
	}
}

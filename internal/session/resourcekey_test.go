package session

import (
	"strings"
	"testing"
)

func TestNewRepoKey(t *testing.T) {
	tests := []struct {
		name     string
		repoName string
		want     string
	}{
		{"simple repo", "devdeploy", "repo:devdeploy"},
		{"repo with dash", "my-repo", "repo:my-repo"},
		{"repo with underscore", "my_repo", "repo:my_repo"},
		{"repo with numbers", "repo123", "repo:repo123"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rk := NewRepoKey(tt.repoName)
			if got := rk.String(); got != tt.want {
				t.Errorf("NewRepoKey(%q).String() = %q, want %q", tt.repoName, got, tt.want)
			}
			if rk.Kind() != "repo" {
				t.Errorf("NewRepoKey(%q).Kind() = %q, want %q", tt.repoName, rk.Kind(), "repo")
			}
			if rk.RepoName() != tt.repoName {
				t.Errorf("NewRepoKey(%q).RepoName() = %q, want %q", tt.repoName, rk.RepoName(), tt.repoName)
			}
			if rk.PRNumber() != 0 {
				t.Errorf("NewRepoKey(%q).PRNumber() = %d, want 0", tt.repoName, rk.PRNumber())
			}
		})
	}
}

func TestNewPRKey(t *testing.T) {
	tests := []struct {
		name     string
		repoName string
		prNumber int
		want     string
	}{
		{"simple PR", "devdeploy", 42, "pr:devdeploy:#42"},
		{"PR with dash repo", "my-repo", 7, "pr:my-repo:#7"},
		{"PR number 1", "grafana", 1, "pr:grafana:#1"},
		{"large PR number", "repo", 9999, "pr:repo:#9999"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rk := NewPRKey(tt.repoName, tt.prNumber)
			if got := rk.String(); got != tt.want {
				t.Errorf("NewPRKey(%q, %d).String() = %q, want %q", tt.repoName, tt.prNumber, got, tt.want)
			}
			if rk.Kind() != "pr" {
				t.Errorf("NewPRKey(%q, %d).Kind() = %q, want %q", tt.repoName, tt.prNumber, rk.Kind(), "pr")
			}
			if rk.RepoName() != tt.repoName {
				t.Errorf("NewPRKey(%q, %d).RepoName() = %q, want %q", tt.repoName, tt.prNumber, rk.RepoName(), tt.repoName)
			}
			if rk.PRNumber() != tt.prNumber {
				t.Errorf("NewPRKey(%q, %d).PRNumber() = %d, want %d", tt.repoName, tt.prNumber, rk.PRNumber(), tt.prNumber)
			}
		})
	}
}

func TestResourceKey_String(t *testing.T) {
	tests := []struct {
		name string
		rk   ResourceKey
		want string
	}{
		{"repo key", NewRepoKey("devdeploy"), "repo:devdeploy"},
		{"PR key", NewPRKey("devdeploy", 42), "pr:devdeploy:#42"},
		{"PR key single digit", NewPRKey("grafana", 7), "pr:grafana:#7"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rk.String(); got != tt.want {
				t.Errorf("ResourceKey.String() = %q, want %q", got, tt.want)
			}
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
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rk.Kind(); got != tt.want {
				t.Errorf("ResourceKey.Kind() = %q, want %q", got, tt.want)
			}
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
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rk.RepoName(); got != tt.wantRepo {
				t.Errorf("ResourceKey.RepoName() = %q, want %q", got, tt.wantRepo)
			}
		})
	}
}

func TestResourceKey_PRNumber(t *testing.T) {
	tests := []struct {
		name     string
		rk       ResourceKey
		wantPR   int
	}{
		{"repo (should be 0)", NewRepoKey("devdeploy"), 0},
		{"PR", NewPRKey("devdeploy", 42), 42},
		{"PR single digit", NewPRKey("grafana", 7), 7},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rk.PRNumber(); got != tt.wantPR {
				t.Errorf("ResourceKey.PRNumber() = %d, want %d", got, tt.wantPR)
			}
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
		{"empty repo name", ResourceKey{kind: "repo", repoName: ""}, false},
		{"empty PR repo name", ResourceKey{kind: "pr", repoName: "", prNumber: 42}, false},
		{"PR with zero number", ResourceKey{kind: "pr", repoName: "devdeploy", prNumber: 0}, false},
		{"PR with negative number", ResourceKey{kind: "pr", repoName: "devdeploy", prNumber: -1}, false},
		{"invalid kind", ResourceKey{kind: "invalid", repoName: "devdeploy"}, false},
		{"unknown kind with valid repo", ResourceKey{kind: "unknown", repoName: "devdeploy", prNumber: 0}, false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rk.IsValid(); got != tt.want {
				t.Errorf("ResourceKey.IsValid() = %v, want %v (key: %+v)", got, tt.want, tt.rk)
			}
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
		{"repo with whitespace trimmed", "  repo:devdeploy  ", NewRepoKey("devdeploy"), false, ""},
		{"PR with whitespace trimmed", "  pr:devdeploy:#42  ", NewPRKey("devdeploy", 42), false, ""},
		
		// Invalid cases
		{"empty string", "", ResourceKey{}, true, "invalid resource key format"},
		{"missing colon", "repodevdeploy", ResourceKey{}, true, "invalid resource key format"},
		{"invalid kind", "invalid:devdeploy", ResourceKey{}, true, "invalid resource key kind"},
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
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseResourceKey(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if err != nil && tt.errMsg != "" {
					if !strings.Contains(err.Error(), tt.errMsg) {
						t.Errorf("ParseResourceKey(%q) error = %q, want error containing %q", tt.input, err.Error(), tt.errMsg)
					}
				}
				return
			}
			if got.String() != tt.want.String() {
				t.Errorf("ParseResourceKey(%q) = %q, want %q", tt.input, got.String(), tt.want.String())
			}
			if got.Kind() != tt.want.Kind() {
				t.Errorf("ParseResourceKey(%q).Kind() = %q, want %q", tt.input, got.Kind(), tt.want.Kind())
			}
			if got.RepoName() != tt.want.RepoName() {
				t.Errorf("ParseResourceKey(%q).RepoName() = %q, want %q", tt.input, got.RepoName(), tt.want.RepoName())
			}
			if got.PRNumber() != tt.want.PRNumber() {
				t.Errorf("ParseResourceKey(%q).PRNumber() = %d, want %d", tt.input, got.PRNumber(), tt.want.PRNumber())
			}
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
		{"repo with dash", NewRepoKey("my-repo")},
		{"PR with dash repo", NewPRKey("my-repo", 123)},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert to string and back
			s := tt.rk.String()
			parsed, err := ParseResourceKey(s)
			if err != nil {
				t.Fatalf("ParseResourceKey(%q) error = %v", s, err)
			}
			
			// Verify all fields match
			if parsed.String() != tt.rk.String() {
				t.Errorf("round trip: got %q, want %q", parsed.String(), tt.rk.String())
			}
			if parsed.Kind() != tt.rk.Kind() {
				t.Errorf("round trip Kind: got %q, want %q", parsed.Kind(), tt.rk.Kind())
			}
			if parsed.RepoName() != tt.rk.RepoName() {
				t.Errorf("round trip RepoName: got %q, want %q", parsed.RepoName(), tt.rk.RepoName())
			}
			if parsed.PRNumber() != tt.rk.PRNumber() {
				t.Errorf("round trip PRNumber: got %d, want %d", parsed.PRNumber(), tt.rk.PRNumber())
			}
		})
	}
}


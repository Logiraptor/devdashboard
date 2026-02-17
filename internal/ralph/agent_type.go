package ralph

import (
	"fmt"

	"devdeploy/internal/beads"
)

// AgentType indicates which agent model and prompt variant to use for a bead.
type AgentType int

const (
	// AgentTypeCoder uses composer-1 with standard prompt.
	AgentTypeCoder AgentType = iota
	// AgentTypeVerifier uses opus with verification prompt.
	AgentTypeVerifier
)

// String returns a human-readable label for the agent type.
func (t AgentType) String() string {
	switch t {
	case AgentTypeCoder:
		return "coder"
	case AgentTypeVerifier:
		return "verifier"
	default:
		return "unknown"
	}
}

// parseAgentType converts a string to an AgentType value.
func parseAgentType(s string) (AgentType, error) {
	switch s {
	case "coder":
		return AgentTypeCoder, nil
	case "verifier":
		return AgentTypeVerifier, nil
	default:
		return 0, ParseEnumError("AgentType", s)
	}
}

// MarshalJSON implements json.Marshaler.
func (t AgentType) MarshalJSON() ([]byte, error) {
	return MarshalEnumJSON(t)
}

// UnmarshalJSON implements json.Unmarshaler.
func (t *AgentType) UnmarshalJSON(data []byte) error {
	parsed, err := UnmarshalEnumJSON(data, parseAgentType)
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

// ReadyBead wraps a beads.Bead with AgentType metadata.
type ReadyBead struct {
	Bead      beads.Bead
	AgentType AgentType
}

// String returns a debug-friendly representation of ReadyBead.
func (rb ReadyBead) String() string {
	return fmt.Sprintf("ReadyBead{ID: %s, Title: %q, AgentType: %s}", rb.Bead.ID, rb.Bead.Title, rb.AgentType)
}

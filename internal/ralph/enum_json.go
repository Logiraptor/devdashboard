package ralph

import (
	"encoding/json"
	"fmt"
)

// StringEnum is a constraint for enum types that have a String() method.
type StringEnum interface {
	String() string
}

// MarshalEnumJSON marshals an enum value to JSON by converting it to its string representation.
// This is a generic helper for implementing json.Marshaler on enum types.
func MarshalEnumJSON[T StringEnum](v T) ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalEnumJSON unmarshals an enum value from JSON by parsing the string representation.
// parseFunc should convert a string to the enum value, or return an error if the string is invalid.
// This is a generic helper for implementing json.Unmarshaler on enum types.
func UnmarshalEnumJSON[T StringEnum](data []byte, parseFunc func(string) (T, error)) (T, error) {
	var zero T
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return zero, err
	}
	return parseFunc(s)
}

// ParseEnumError creates a standardized error message for invalid enum string values.
func ParseEnumError(enumName, value string) error {
	return fmt.Errorf("unknown %s: %s", enumName, value)
}

// Package jsonutil provides shared utilities for JSON parsing patterns:
// error handling, type conversion, and validation helpers.
package jsonutil

import (
	"encoding/json"
	"fmt"
)

// UnmarshalWithContext unmarshals JSON data into v and wraps any error
// with the provided context message.
func UnmarshalWithContext(data []byte, v interface{}, context string) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", context, err)
	}
	return nil
}

// GetString safely extracts a string value from a map[string]interface{}.
// Returns the value if it's a string, otherwise returns empty string.
func GetString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// GetStringOr safely extracts a string value from a map[string]interface{}
// with a default value if the key doesn't exist or isn't a string.
func GetStringOr(m map[string]interface{}, key string, defaultValue string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return defaultValue
}

// ToString converts an interface{} value to a string representation.
// Handles string, float64 (formatted as integer), bool, and other types.
func ToString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		// Format as integer for whole numbers, otherwise as float
		if val == float64(int64(val)) {
			return fmt.Sprintf("%.0f", val)
		}
		return fmt.Sprintf("%g", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// UnmarshalArray unmarshals JSON data into a slice and validates that
// the result is non-empty. Returns an error if unmarshaling fails or
// the array is empty.
func UnmarshalArray[T any](data []byte, context string) ([]T, error) {
	var entries []T
	if err := UnmarshalWithContext(data, &entries, context); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s: empty result", context)
	}
	return entries, nil
}

// UnmarshalArrayAllowEmpty unmarshals JSON data into a slice.
// Unlike UnmarshalArray, this allows empty arrays.
func UnmarshalArrayAllowEmpty[T any](data []byte, context string) ([]T, error) {
	var entries []T
	if err := UnmarshalWithContext(data, &entries, context); err != nil {
		return nil, err
	}
	return entries, nil
}

// UnmarshalLine unmarshals a single JSON line (string) into v.
// Returns an error if the line is empty or cannot be parsed.
func UnmarshalLine(line string, v interface{}) error {
	if line == "" {
		return fmt.Errorf("empty JSON line")
	}
	return json.Unmarshal([]byte(line), v)
}

// UnmarshalLineSafe unmarshals a single JSON line (string) into v.
// Returns false if the line is empty or cannot be parsed, true on success.
// Useful when parsing multiple lines where some may be invalid.
func UnmarshalLineSafe(line string, v interface{}) bool {
	return UnmarshalLine(line, v) == nil
}

package jsonutil

import (
	"testing"
)

func TestUnmarshalWithContext(t *testing.T) {
	type TestStruct struct {
		Name string `json:"name"`
	}

	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "valid JSON",
			data:    []byte(`{"name":"test"}`),
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			data:    []byte(`not json`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v TestStruct
			err := UnmarshalWithContext(tt.data, &v, "test context")
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalWithContext() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && v.Name != "test" {
				t.Errorf("UnmarshalWithContext() v.Name = %q, want %q", v.Name, "test")
			}
		})
	}
}

func TestGetString(t *testing.T) {
	m := map[string]interface{}{
		"str":    "value",
		"num":    42.0,
		"bool":   true,
		"nil":    nil,
		"missing": nil,
	}

	tests := []struct {
		key  string
		want string
	}{
		{"str", "value"},
		{"num", ""},
		{"bool", ""},
		{"nil", ""},
		{"missing", ""},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := GetString(m, tt.key); got != tt.want {
				t.Errorf("GetString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetStringOr(t *testing.T) {
	m := map[string]interface{}{
		"str": "value",
		"num": 42.0,
	}

	tests := []struct {
		key          string
		defaultValue string
		want         string
	}{
		{"str", "default", "value"},
		{"num", "default", "default"},
		{"missing", "default", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := GetStringOr(m, tt.key, tt.defaultValue); got != tt.want {
				t.Errorf("GetStringOr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestToString(t *testing.T) {
	tests := []struct {
		name string
		v    interface{}
		want string
	}{
		{"string", "hello", "hello"},
		{"float64 whole", 42.0, "42"},
		{"float64 decimal", 3.14, "3.14"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"nil", nil, ""},
		{"int", 123, "123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToString(tt.v); got != tt.want {
				t.Errorf("ToString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUnmarshalArray(t *testing.T) {
	type TestStruct struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	tests := []struct {
		name    string
		data    []byte
		wantErr bool
		wantLen int
	}{
		{
			name:    "valid non-empty array",
			data:    []byte(`[{"id":1,"name":"test"}]`),
			wantErr: false,
			wantLen: 1,
		},
		{
			name:    "empty array",
			data:    []byte(`[]`),
			wantErr: true,
			wantLen: 0,
		},
		{
			name:    "invalid JSON",
			data:    []byte(`not json`),
			wantErr: true,
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UnmarshalArray[TestStruct](tt.data, "test context")
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalArray() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != tt.wantLen {
				t.Errorf("UnmarshalArray() len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestUnmarshalArrayAllowEmpty(t *testing.T) {
	type TestStruct struct {
		ID int `json:"id"`
	}

	tests := []struct {
		name    string
		data    []byte
		wantErr bool
		wantLen int
	}{
		{
			name:    "valid non-empty array",
			data:    []byte(`[{"id":1}]`),
			wantErr: false,
			wantLen: 1,
		},
		{
			name:    "empty array",
			data:    []byte(`[]`),
			wantErr: false,
			wantLen: 0,
		},
		{
			name:    "invalid JSON",
			data:    []byte(`not json`),
			wantErr: true,
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UnmarshalArrayAllowEmpty[TestStruct](tt.data, "test context")
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalArrayAllowEmpty() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != tt.wantLen {
				t.Errorf("UnmarshalArrayAllowEmpty() len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestUnmarshalLine(t *testing.T) {
	type TestStruct struct {
		Value string `json:"value"`
	}

	tests := []struct {
		name    string
		line    string
		wantErr bool
		want    string
	}{
		{
			name:    "valid JSON line",
			line:    `{"value":"test"}`,
			wantErr: false,
			want:    "test",
		},
		{
			name:    "empty line",
			line:    "",
			wantErr: true,
			want:    "",
		},
		{
			name:    "invalid JSON",
			line:    `not json`,
			wantErr: true,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v TestStruct
			err := UnmarshalLine(tt.line, &v)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && v.Value != tt.want {
				t.Errorf("UnmarshalLine() v.Value = %q, want %q", v.Value, tt.want)
			}
		})
	}
}

func TestUnmarshalLineSafe(t *testing.T) {
	type TestStruct struct {
		Value string `json:"value"`
	}

	tests := []struct {
		name string
		line string
		want bool
	}{
		{"valid JSON", `{"value":"test"}`, true},
		{"empty line", "", false},
		{"invalid JSON", `not json`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v TestStruct
			if got := UnmarshalLineSafe(tt.line, &v); got != tt.want {
				t.Errorf("UnmarshalLineSafe() = %v, want %v", got, tt.want)
			}
		})
	}
}

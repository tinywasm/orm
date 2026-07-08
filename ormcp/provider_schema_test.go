package ormcp

import (
	"testing"

	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
)

func TestEncodeSchema(t *testing.T) {
	tests := []struct {
		name     string
		input    model.Fielder
		expected string
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: EmptyInputSchema,
		},
		{
			name:     "QueryArgs",
			input:    new(QueryArgs),
			expected: `{"type":"object","properties":{"SQL":{"type":"string"}}}`,
		},
		{
			name:     "ExecArgs",
			input:    new(ExecArgs),
			expected: `{"type":"object","properties":{"SQL":{"type":"string"}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encodeSchema(tt.input)
			if got != tt.expected {
				t.Errorf("encodeSchema() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestToolsSchemaValidity(t *testing.T) {
	exec := &mockInspector{}
	db := orm.New(exec, nil)
	p := NewProvider(db)
	dp := NewDaemonProvider()

	allTools := append(p.Tools(), dp.Tools()...)

	for _, tool := range allTools {
		t.Run(tool.Name, func(t *testing.T) {
			if tool.InputSchema == "" {
				t.Errorf("tool %s has empty InputSchema", tool.Name)
			}
			if tool.InputSchema == "null" {
				t.Errorf("tool %s has null InputSchema", tool.Name)
			}
			// Since we can't easily use tinywasm/json for maps without reflection/structs,
			// and we verified encodeSchema logic, we just check basic JSON-like structure here.
			if tool.InputSchema[0] != '{' || tool.InputSchema[len(tool.InputSchema)-1] != '}' {
				t.Errorf("tool %s InputSchema does not look like a JSON object: %s", tool.Name, tool.InputSchema)
			}
		})
	}
}

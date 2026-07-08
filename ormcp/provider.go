package ormcp

import "github.com/tinywasm/model"

import (
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/mcp"
	"github.com/tinywasm/orm"
)

// Provider implements mcp.ToolProvider for a live *orm.DB connection.
type Provider struct {
	db *orm.DB
}

// NewProvider creates a new MCP tool provider wrapping the given DB.
func NewProvider(db *orm.DB) *Provider {
	return &Provider{db: db}
}

// Tools returns the MCP tools available for this DB connection.
// db_schema is only included if the underlying executor implements orm.SchemaInspector.
func (p *Provider) Tools() []mcp.Tool {
	tools := []mcp.Tool{
		queryTool(p.db),
		execTool(p.db),
		exportTool(p.db),
	}
	if _, ok := p.db.RawExecutor().(orm.SchemaInspector); ok {
		tools = append([]mcp.Tool{schemaTool(p.db)}, tools...)
	}
	return tools
}

// EmptyInputSchema is the JSON Schema for a tool that takes no arguments.
// MCP clients require inputSchema to be a valid JSON Schema object; an empty
// string or null is rejected and causes the ENTIRE tools/list to be discarded
// (Claude Code validates tools/list with Zod).
const EmptyInputSchema = `{"type":"object","properties":{}}`

// encodeSchema builds a valid JSON Schema "object" string for an MCP tool's
// inputSchema, derived from the args model's Schema() field metadata. Replaces
// the previous broken behavior that json-encoded the struct's zero values
// (e.g. {"SQL":""}), which is NOT a JSON Schema and is rejected by MCP clients.
// Returns EmptyInputSchema for a nil model or one with no fields.
func encodeSchema(m model.Fielder) string {
	if m == nil {
		return EmptyInputSchema
	}
	fields := m.Schema()
	if len(fields) == 0 {
		return EmptyInputSchema
	}
	var b fmt.Conv
	b.Write(`{"type":"object","properties":{`)
	var required []string
	for i, f := range fields {
		if i > 0 {
			b.Write(",")
		}
		b.Write(`"`)
		b.Write(f.Name)
		b.Write(`":`)
		b.Write(jsonSchemaType(f.Type))
		if f.NotNull {
			required = append(required, f.Name)
		}
	}
	b.Write("}")
	if len(required) > 0 {
		b.Write(`,"required":[`)
		for i, name := range required {
			if i > 0 {
				b.Write(",")
			}
			b.Write(`"`)
			b.Write(name)
			b.Write(`"`)
		}
		b.Write("]")
	}
	b.Write("}")
	return b.String()
}

// jsonSchemaType maps a model.FieldType to its JSON Schema fragment.
//
//	FieldText, FieldRaw, FieldBlob -> string
//	FieldInt                       -> integer
//	FieldFloat                     -> number
//	FieldBool                      -> boolean
//	FieldIntSlice                  -> array of integer
//	FieldStruct                    -> object
//	FieldStructSlice               -> array of object
func jsonSchemaType(t model.FieldType) string {
	switch t {
	case model.FieldInt:
		return `{"type":"integer"}`
	case model.FieldFloat:
		return `{"type":"number"}`
	case model.FieldBool:
		return `{"type":"boolean"}`
	case model.FieldIntSlice:
		return `{"type":"array","items":{"type":"integer"}}`
	case model.FieldStruct:
		return `{"type":"object"}`
	case model.FieldStructSlice:
		return `{"type":"array","items":{"type":"object"}}`
	default: // FieldText, FieldRaw, FieldBlob
		return `{"type":"string"}`
	}
}

func scanRowsToText(rows orm.Rows) string {
	cols, _ := rows.Columns()
	var out fmt.Conv
	out.Write(fmt.Convert(cols).Join(" | ").String() + "\n")
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		rows.Scan(ptrs...)
		parts := make([]string, len(vals))
		for i, v := range vals {
			parts[i] = fmt.Sprint(v)
		}
		out.Write(fmt.Convert(parts).Join(" | ").String() + "\n")
	}
	if err := rows.Err(); err != nil {
		out.Write("error: " + err.Error())
	}
	return out.String()
}

package ormcp

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

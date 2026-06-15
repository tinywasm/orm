//go:build !wasm

package mcporm

import (
    "github.com/tinywasm/context"
    "github.com/tinywasm/fmt"
    "github.com/tinywasm/mcp"
    "github.com/tinywasm/orm"
)

func schemaTool(db *orm.DB) mcp.Tool {
    return mcp.Tool{
        Name:        "db_schema",
        Description: "List all tables and their columns with types and constraints. Use this first to understand the database structure before writing queries.",
        InputSchema: "",  // no input args
        Resource:    "database",
        Action:      'r',
        Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
            inspector := db.RawExecutor().(orm.SchemaInspector)

            tables, err := inspector.Tables()
            if err != nil {
                return nil, err
            }

            var out fmt.Conv
            for _, table := range tables {
                out.Write(table + ":\n")
                cols, err := inspector.Columns(table)
                if err != nil {
                    out.Write("  (error reading columns: " + err.Error() + ")\n")
                    continue
                }
                for _, col := range cols {
                    line := "  " + col.Name + " " + col.Type
                    if col.PK {
                        line += " PK"
                    }
                    if col.NotNull {
                        line += " NOT NULL"
                    }
                    out.Write(line + "\n")
                }
            }
            return mcp.Text(out.String()), nil
        },
    }
}

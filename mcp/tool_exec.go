//go:build !wasm

package mcporm

import (
    "encoding/json"

    "github.com/tinywasm/context"
    "github.com/tinywasm/fmt"
    "github.com/tinywasm/mcp"
    "github.com/tinywasm/orm"
)

func execTool(db *orm.DB) mcp.Tool {
    return mcp.Tool{
        Name:        "db_exec",
        Description: "Execute a SQL statement that modifies data or schema: INSERT, UPDATE, DELETE, CREATE TABLE, ALTER TABLE, DROP TABLE, etc.",
        InputSchema: encodeSchema(new(ExecArgs)),
        Resource:    "database",
        Action:      'u',
        Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
            var args ExecArgs
            if err := req.Bind(&args); err != nil {
                return nil, err
            }

            var queryArgs []any
            if args.Args != "" {
                if err := json.Unmarshal([]byte(args.Args), &queryArgs); err != nil {
                    return nil, fmt.Err("failed to decode exec args: %v", err)
                }
            }

            if err := db.RawExecutor().Exec(args.SQL, queryArgs...); err != nil {
                return nil, err
            }
            return mcp.Text("OK"), nil
        },
    }
}

//go:build !wasm

package mcporm

import (
    "encoding/json"
    "strings"

    "github.com/tinywasm/context"
    "github.com/tinywasm/fmt"
    "github.com/tinywasm/mcp"
    "github.com/tinywasm/orm"
)

func queryTool(db *orm.DB) mcp.Tool {
    return mcp.Tool{
        Name:        "db_query",
        Description: "Execute a read-only SQL query (SELECT/WITH) and return the results as text. Use db_exec for INSERT, UPDATE, DELETE, or DDL.",
        InputSchema: encodeSchema(new(QueryArgs)),
        Resource:    "database",
        Action:      'r',
        Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
            var args QueryArgs
            if err := req.Bind(&args); err != nil {
                return nil, err
            }
            upper := strings.ToUpper(strings.TrimSpace(args.SQL))
            if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
                return nil, fmt.Err("db_query only accepts SELECT or WITH statements; use db_exec for mutations")
            }

            var queryArgs []any
            if args.Args != "" {
                if err := json.Unmarshal([]byte(args.Args), &queryArgs); err != nil {
                    return nil, fmt.Err("failed to decode query args: %v", err)
                }
            }

            rows, err := db.RawExecutor().Query(args.SQL, queryArgs...)
            if err != nil {
                return nil, err
            }
            defer rows.Close()

            return mcp.Text(scanRowsToText(rows)), nil
        },
    }
}

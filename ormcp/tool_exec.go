package ormcp

import (
	"github.com/tinywasm/context"
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
			if err := db.RawExecutor().Exec(args.SQL); err != nil {
				return nil, err
			}
			return mcp.Text("OK"), nil
		},
	}
}

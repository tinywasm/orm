package ormcp

import (
	"github.com/tinywasm/context"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/mcp"
	"github.com/tinywasm/orm"
)

func execTool(db *orm.DB) mcp.Tool {
	return mcp.Tool{
		Name:        "db_exec",
		Description: "Execute a SQL statement that modifies data or schema: INSERT, UPDATE, DELETE, CREATE TABLE, ALTER TABLE, DROP TABLE, etc.",
		Args:        new(ExecArgs),
		Resource:    "database",
		Action:      'u',
		Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
			return executeExec(db, req)
		},
	}
}

func executeExec(db *orm.DB, req mcp.Request) (*mcp.Result, error) {
	if db == nil {
		return nil, fmt.Err("no database configured: call start_development first")
	}
	var args ExecArgs
	if err := req.Bind(&args); err != nil {
		return nil, err
	}
	if err := db.RawExecutor().Exec(args.Sql); err != nil {
		return nil, err
	}
	return mcp.Text("OK"), nil
}

package ormcp

import (
	"github.com/tinywasm/context"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/mcp"
	"github.com/tinywasm/orm"
)

func queryTool(db *orm.DB) mcp.Tool {
	return mcp.Tool{
		Name:        "db_query",
		Description: "Execute a read-only SQL query (SELECT/WITH) and return the results as text. Use db_exec for INSERT, UPDATE, DELETE, or DDL.",
		Args:        new(QueryArgs),
		Resource:    "database",
		Action:      'r',
		Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
			return executeQuery(db, req)
		},
	}
}

func executeQuery(db *orm.DB, req mcp.Request) (*mcp.Result, error) {
	if db == nil {
		return nil, fmt.Err("no database configured: call start_development first")
	}
	var args QueryArgs
	if err := req.Bind(&args); err != nil {
		return nil, err
	}
	upper := fmt.Convert(args.Sql).TrimSpace().ToUpper().String()
	if !fmt.HasPrefix(upper, "SELECT") && !fmt.HasPrefix(upper, "WITH") {
		return nil, fmt.Err("db_query only accepts SELECT or WITH statements; use db_exec for mutations")
	}
	rows, err := db.RawExecutor().Query(args.Sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return mcp.Text(scanRowsToText(rows)), nil
}

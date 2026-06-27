package ormcp

import (
	"github.com/tinywasm/context"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/mcp"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/orm/ddl"
)

func exportSchemaTool(db *orm.DB) mcp.Tool {
	return mcp.Tool{
		Name:        "db_export_schema",
		Description: "Export the full CREATE TABLE DDL for all synced tables as SQL text.",
		Resource:    "database",
		Action:      'r',
		Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
			return executeExportSchema(db, req)
		},
	}
}

func executeExportSchema(db *orm.DB, _ mcp.Request) (*mcp.Result, error) {
	if db == nil {
		return nil, fmt.Err("no database configured: call start_development first")
	}
	exporter, ok := db.Compiler().(ddl.Exporter)
	if !ok {
		return nil, fmt.Err("adapter does not support DDL export")
	}
	models := db.RegisteredModels()
	if len(models) == 0 {
		return mcp.Text("-- no tables registered"), nil
	}
	sql, err := exporter.ExportDDL(models)
	if err != nil {
		return nil, err
	}
	return mcp.Text(sql), nil
}

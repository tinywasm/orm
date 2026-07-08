//go:build !wasm

package ormcp

import (
	"sync"

	"github.com/tinywasm/context"
	"github.com/tinywasm/mcp"
	"github.com/tinywasm/orm"
)

// DaemonProvider implements mcp.ToolProvider for the MCP daemon.
// Tools are registered at startup; SetDB wires the live connection at runtime.
type DaemonProvider struct {
	mu sync.RWMutex
	db *orm.DB
}

// NewDaemonProvider creates a new DaemonProvider.
func NewDaemonProvider() *DaemonProvider { return &DaemonProvider{} }

// SetDB swaps the active DB. Call with nil when the project stops.
func (p *DaemonProvider) SetDB(db *orm.DB) {
	p.mu.Lock()
	p.db = db
	p.mu.Unlock()
}

// Tools returns db_schema (always), db_query, db_exec — fixed schemas, no DB required.
func (p *DaemonProvider) Tools() []mcp.Tool {
	return []mcp.Tool{
		p.schemaToolD(),
		p.queryToolD(),
		p.execToolD(),
		p.exportToolD(),
	}
}

func (p *DaemonProvider) schemaToolD() mcp.Tool {
	return mcp.Tool{
		Name:        "db_schema",
		Description: "List all tables and their columns with types and constraints. Use this first to understand the database structure before writing queries.",
		Resource:    "database",
		Action:      'r',
		Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
			p.mu.RLock()
			db := p.db
			p.mu.RUnlock()
			return executeSchema(db, req)
		},
	}
}

func (p *DaemonProvider) exportToolD() mcp.Tool {
	return mcp.Tool{
		Name:        "db_export_schema",
		Description: "Export the full CREATE TABLE DDL for all synced tables as SQL text.",
		Resource:    "database",
		Action:      'r',
		Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
			p.mu.RLock()
			db := p.db
			p.mu.RUnlock()
			return executeExportSchema(db, req)
		},
	}
}

func (p *DaemonProvider) queryToolD() mcp.Tool {
	return mcp.Tool{
		Name:        "db_query",
		Description: "Execute a read-only SQL query (SELECT/WITH) and return the results as text. Use db_exec for INSERT, UPDATE, DELETE, or DDL.",
		Args:        new(QueryArgs),
		Resource:    "database",
		Action:      'r',
		Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
			p.mu.RLock()
			db := p.db
			p.mu.RUnlock()
			return executeQuery(db, req)
		},
	}
}

func (p *DaemonProvider) execToolD() mcp.Tool {
	return mcp.Tool{
		Name:        "db_exec",
		Description: "Execute a SQL statement that modifies data or schema: INSERT, UPDATE, DELETE, CREATE TABLE, ALTER TABLE, DROP TABLE, etc.",
		Args:        new(ExecArgs),
		Resource:    "database",
		Action:      'u',
		Execute: func(ctx *context.Context, req mcp.Request) (*mcp.Result, error) {
			p.mu.RLock()
			db := p.db
			p.mu.RUnlock()
			return executeExec(db, req)
		},
	}
}

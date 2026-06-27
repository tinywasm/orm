package ormcp

import (
	"strings"
	"testing"

	"github.com/tinywasm/context"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/json"
	"github.com/tinywasm/mcp"
	"github.com/tinywasm/orm"
)

func TestDaemonProvider_Tools(t *testing.T) {
	p := NewDaemonProvider()
	tools := p.Tools()
	if len(tools) != 4 {
		t.Errorf("expected 4 tools, got %d", len(tools))
	}

	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}

	for _, name := range []string{"db_schema", "db_query", "db_exec", "db_export_schema"} {
		if !names[name] {
			t.Errorf("missing tool %s", name)
		}
	}
}

func TestDaemonProvider_NoDB(t *testing.T) {
	p := NewDaemonProvider()
	ctx := context.Background()

	for _, tool := range p.Tools() {
		_, err := tool.Execute(ctx, mcp.Request{})
		if err == nil || !strings.Contains(err.Error(), "no database configured") {
			t.Errorf("tool %s: expected 'no database configured' error, got %v", tool.Name, err)
		}
	}
}

type mockExecutor struct {
	orm.Executor
	queryCalled bool
	execCalled  bool
}

func (m *mockExecutor) Query(query string, args ...any) (orm.Rows, error) {
	m.queryCalled = true
	return &mockRows{}, nil
}

func (m *mockExecutor) Exec(query string, args ...any) error {
	m.execCalled = true
	return nil
}

func (m *mockExecutor) Close() error { return nil }

type mockRows struct {
	orm.Rows
}

func (m *mockRows) Columns() ([]string, error) { return []string{"col1"}, nil }
func (m *mockRows) Next() bool                { return false }
func (m *mockRows) Close() error               { return nil }
func (m *mockRows) Err() error                 { return nil }

type mockInspector struct {
	mockExecutor
}

func (m *mockInspector) Tables() ([]string, error) { return []string{"users"}, nil }
func (m *mockInspector) Columns(table string) ([]orm.ColumnInfo, error) {
	return []orm.ColumnInfo{{Name: "id", Type: "INTEGER", PK: true}}, nil
}

func TestDaemonProvider_WithDB(t *testing.T) {
	p := NewDaemonProvider()
	exec := &mockInspector{}
	db := orm.New(exec, nil)
	p.SetDB(db)

	ctx := context.Background()

	// Test db_query
	reqQuery := mcp.Request{Params: mcp.CallToolParams{Arguments: `{"SQL": "SELECT 1"}`}}
	_, err := p.Tools()[1].Execute(ctx, reqQuery) // db_query is at index 1
	if err != nil {
		t.Errorf("db_query failed: %v", err)
	}
	if !exec.queryCalled {
		t.Errorf("db_query did not call executor.Query")
	}

	// Test db_exec
	reqExec := mcp.Request{Params: mcp.CallToolParams{Arguments: `{"SQL": "UPDATE x SET y=1"}`}}
	_, err = p.Tools()[2].Execute(ctx, reqExec) // db_exec is at index 2
	if err != nil {
		t.Errorf("db_exec failed: %v", err)
	}
	if !exec.execCalled {
		t.Errorf("db_exec did not call executor.Exec")
	}

	// Test db_schema
	_, err = p.Tools()[0].Execute(ctx, mcp.Request{}) // db_schema is at index 0
	if err != nil {
		t.Errorf("db_schema failed: %v", err)
	}

	// Test SetDB(nil)
	p.SetDB(nil)
	_, err = p.Tools()[1].Execute(ctx, reqQuery)
	if err == nil || !strings.Contains(err.Error(), "no database configured") {
		t.Errorf("expected error after SetDB(nil), got %v", err)
	}
}

type mockCompiler struct {
	orm.Compiler
}

func (m *mockCompiler) Compile(q orm.Query, model fmt.Model) (orm.Plan, error) {
	return orm.Plan{}, nil
}

type mockExporterCompiler struct {
	mockCompiler
}

func (m *mockExporterCompiler) ExportDDL(models []fmt.Model) (string, error) {
	return "CREATE TABLE export", nil
}

type mockModel struct {
	name string
}

func (m *mockModel) ModelName() string           { return m.name }
func (m *mockModel) Schema() []fmt.Field         { return nil }
func (m *mockModel) Pointers() []any             { return nil }
func (m *mockModel) IsNil() bool                 { return m == nil }
func (m *mockModel) EncodeFields(fmt.FieldWriter) {}
func (m *mockModel) DecodeFields(fmt.FieldReader) {}

func TestExportSchema_WithAdapter(t *testing.T) {
	p := NewDaemonProvider()
	compiler := &mockExporterCompiler{}
	db := orm.New(&mockExecutor{}, compiler)
	db.Sync(&mockModel{name: "users"})
	p.SetDB(db)

	ctx := context.Background()
	res, err := p.Tools()[3].Execute(ctx, mcp.Request{}) // db_export_schema is at index 3
	if err != nil {
		t.Fatalf("db_export_schema failed: %v", err)
	}
	// Check if the result contains the expected SQL.
	var s string
	json.Encode(res, &s)
	if !strings.Contains(s, "CREATE TABLE export") {
		t.Errorf("unexpected export result: %s", s)
	}
}

func TestExportSchema_NoAdapter(t *testing.T) {
	p := NewDaemonProvider()
	db := orm.New(&mockExecutor{}, &mockCompiler{}) // mockCompiler does not implement Exporter
	p.SetDB(db)

	ctx := context.Background()
	_, err := p.Tools()[3].Execute(ctx, mcp.Request{})
	if err == nil || !strings.Contains(err.Error(), "adapter does not support") {
		t.Errorf("expected 'adapter does not support' error, got %v", err)
	}
}

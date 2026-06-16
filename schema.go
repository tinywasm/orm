package orm

// SchemaInspector is optionally implemented by Executor adapters to expose
// database schema introspection. If the adapter does not implement it,
// the db_schema MCP tool is not registered.
type SchemaInspector interface {
    Tables() ([]string, error)
    Columns(table string) ([]ColumnInfo, error)
}

// ColumnInfo describes a single column returned by SchemaInspector.
type ColumnInfo struct {
    Name    string
    Type    string
    NotNull bool
    PK      bool
}

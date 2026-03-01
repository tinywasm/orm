package orm

// Model represents a database model.
// Consumers implement this interface.
type Model interface {
	TableName() string
	Schema() []Field
	Values() []any
	Pointers() []any
}

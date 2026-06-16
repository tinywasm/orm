package ormcp

import "github.com/tinywasm/fmt"

type QueryArgs struct {
	SQL string
}

func (m *QueryArgs) Schema() []fmt.Field {
	return []fmt.Field{
		{Name: "SQL", Type: fmt.FieldText, NotNull: true},
	}
}

func (m *QueryArgs) Validate(action byte) error { return fmt.ValidateFields(action, m) }
func (m *QueryArgs) Pointers() []any            { return []any{&m.SQL} }

type ExecArgs struct {
	SQL string
}

func (m *ExecArgs) Schema() []fmt.Field {
	return []fmt.Field{
		{Name: "SQL", Type: fmt.FieldText, NotNull: true},
	}
}

func (m *ExecArgs) Validate(action byte) error { return fmt.ValidateFields(action, m) }
func (m *ExecArgs) Pointers() []any            { return []any{&m.SQL} }

package ormcp

import "github.com/tinywasm/fmt"

type QueryArgs struct {
	SQL string `json:"SQL"`
}

func (m *QueryArgs) Validate(_ byte) error {
	if m.SQL == "" {
		return fmt.Err("SQL", "required")
	}
	return nil
}

type ExecArgs struct {
	SQL string `json:"SQL"`
}

func (m *ExecArgs) Validate(_ byte) error {
	if m.SQL == "" {
		return fmt.Err("SQL", "required")
	}
	return nil
}

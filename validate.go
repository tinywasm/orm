package orm

import "github.com/tinywasm/fmt"

func validate(action Action, m Model) error {
	if m.TableName() == "" {
		return ErrEmptyTable
	}

	if action == ActionCreate || action == ActionUpdate {
		if len(m.Columns()) != len(m.Values()) {
			return fmt.Err(ErrValidation, "columns and values length mismatch")
		}
	}

	return nil
}

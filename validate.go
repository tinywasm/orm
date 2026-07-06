package orm

import "github.com/tinywasm/model"

import "github.com/tinywasm/fmt"

func validateQuery(action Action, m model.Model) error {
	if action != ActionCreateDatabase && m.ModelName() == "" {
		return ErrEmptyTable
	}

	if action == ActionCreate || action == ActionUpdate {
		if len(m.Schema()) != len(m.Pointers()) {
			return fmt.Err(ErrValidation, "schema and pointers length mismatch")
		}
	}

	return nil
}

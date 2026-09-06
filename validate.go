package orm

import (
	"webtyp.com/fmt"
	"webtyp.com/model"
	"webtyp.com/storage"
)

func validateQuery(action storage.Action, m model.Model) error {
	if m.ModelName() == "" {
		return ErrEmptyTable
	}
	if action == storage.ActionCreate || action == storage.ActionUpdate {
		if len(m.Schema()) != len(m.Pointers()) {
			return fmt.Err(ErrValidation, "schema and pointers length mismatch")
		}
	}
	return nil
}

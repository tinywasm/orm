package ormcp

import "github.com/tinywasm/model"

var QueryArgsModel = model.Definition{
	Name: "query_args",
	Fields: model.Fields{
		{Name: "SQL", Type: model.FieldText, NotNull: true},
	},
}

var ExecArgsModel = model.Definition{
	Name: "exec_args",
	Fields: model.Fields{
		{Name: "SQL", Type: model.FieldText, NotNull: true},
	},
}

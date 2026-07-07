package tests

import (
	"github.com/tinywasm/model"
)

var UserModel = model.Definition{
	Name: "user",
	Fields: model.Fields{
		{Name: "id", Type: model.FieldInt, DB: &model.FieldDB{PK: true}},
		{Name: "first_name", Type: model.FieldText, NotNull: true},
		{Name: "last_name", Type: model.FieldText},
		{Name: "email", Type: model.FieldText, DB: &model.FieldDB{Unique: true}},
		{Name: "score", Type: model.FieldFloat},
		{Name: "is_active", Type: model.FieldBool},
		{Name: "avatar", Type: model.FieldBlob},
	},
}

var OrderModel = model.Definition{
	Name: "order",
	Fields: model.Fields{
		{Name: "id", Type: model.FieldText, DB: &model.FieldDB{PK: true}},
		{Name: "user_id", Type: model.FieldInt, Ref: &UserModel, DB: &model.FieldDB{RefColumn: "id"}},
		{Name: "total", Type: model.FieldFloat},
	},
}

var ModelWithIgnoredModel = model.Definition{
	Name: "model_with_ignored",
	Fields: model.Fields{
		{Name: "id", Type: model.FieldText, DB: &model.FieldDB{PK: true}},
		{Name: "name", Type: model.FieldText},
		{Name: "tags", Type: model.FieldText, Exclude: true}, // Exclude replaces the need for db:"-" for in-memory fields
		{Name: "score", Type: model.FieldFloat},
	},
}

var MultiAModel = model.Definition{
	Name: "multi_a_records",
	Fields: model.Fields{
		{Name: "id", Type: model.FieldText, DB: &model.FieldDB{PK: true}},
		{Name: "name", Type: model.FieldText},
	},
}

var MultiBModel = model.Definition{
	Name: "multi_b",
	Fields: model.Fields{
		{Name: "id", Type: model.FieldText, DB: &model.FieldDB{PK: true}},
		{Name: "value", Type: model.FieldInt},
	},
}

var NumericTypesModel = model.Definition{
	Name: "numeric_types",
	Fields: model.Fields{
		{Name: "id_numeric", Type: model.FieldInt, DB: &model.FieldDB{PK: true}},
		{Name: "count_uint", Type: model.FieldInt},
		{Name: "ratio_f32", Type: model.FieldFloat},
	},
}

var RefNoColumnModel = model.Definition{
	Name: "ref_no_column",
	Fields: model.Fields{
		{Name: "id_ref", Type: model.FieldText, DB: &model.FieldDB{PK: true}},
		{Name: "parent_id", Type: model.FieldInt, Ref: &MultiAModel},
	},
}

var PointerReceiverModel = model.Definition{
	Name: "ptr_table",
	Fields: model.Fields{
		{Name: "id", Type: model.FieldText, DB: &model.FieldDB{PK: true}},
		{Name: "name", Type: model.FieldText},
	},
}

var UserFormModel = model.Definition{
	Name: "user_form",
	Fields: model.Fields{
		{Name: "id", Type: model.FieldText, DB: &model.FieldDB{PK: true}},
		{Name: "name", Type: model.FieldText, Permitted: model.Permitted{Minimum: 2, Maximum: 100}},
		{Name: "email", Type: model.FieldText, NotNull: true},
		{Name: "password", Type: model.FieldText, NotNull: true, Permitted: model.Permitted{Minimum: 8}},
		{Name: "bio", Type: model.FieldText, Permitted: model.Permitted{Tilde: true, Spaces: true}},
		{Name: "age", Type: model.FieldInt},
	},
}

var LoginFormModel = model.Definition{
	Name: "login_form",
	Fields: model.Fields{
		{Name: "email", Type: model.FieldText, NotNull: true},
		{Name: "password", Type: model.FieldText, NotNull: true},
	},
}

var AddressModel = model.Definition{
	Name: "address",
	Fields: model.Fields{
		{Name: "street", Type: model.FieldText},
		{Name: "city", Type: model.FieldText},
	},
}

var UserWithCompositionModel = model.Definition{
	Name: "user_with_composition",
	Fields: model.Fields{
		{Name: "id", Type: model.FieldText, DB: &model.FieldDB{PK: true}},
		{Name: "name", Type: model.FieldText},
		{Name: "home_addr", Type: model.FieldStruct, Ref: &AddressModel},
	},
}

var UserWithNoTildeModel = model.Definition{
	Name: "user_with_no_tilde",
	Fields: model.Fields{
		{Name: "id", Type: model.FieldText, DB: &model.FieldDB{PK: true}},
		{Name: "nombre", Type: model.FieldText, NotNull: true},
	},
}

var ShortAutoIncModel = model.Definition{
	Name: "short_auto_inc",
	Fields: model.Fields{
		{Name: "id", Type: model.FieldInt, DB: &model.FieldDB{PK: true, AutoInc: true}},
		{Name: "value", Type: model.FieldInt},
	},
}

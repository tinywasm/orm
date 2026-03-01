package orm

// FieldType represents the abstract storage type of a model field.
type FieldType int

const (
	TypeText FieldType = iota
	TypeInt64
	TypeFloat64
	TypeBool
	TypeBlob
)

// Constraint is a bitmask of column-level constraints.
// ConstraintNone = 0 is defined separately to avoid shifting iota off-by-one.
type Constraint int

const ConstraintNone Constraint = 0

const (
	ConstraintPK            Constraint = 1 << iota // 1: Primary Key (auto-detected via fmt.IDorPrimaryKey)
	ConstraintUnique                               // 2: UNIQUE
	ConstraintNotNull                              // 4: NOT NULL
	ConstraintAutoIncrement                        // 8: SERIAL / AUTOINCREMENT / {autoIncrement: true}
)

// Field describes a single column in a model's schema.
// Schema() and Values() MUST always be in the same field order.
// Field.Ref is present in all adapters for API compatibility; adapters that
// do not support FKs (e.g. IndexedDB) silently ignore it without error.
type Field struct {
	Name        string
	Type        FieldType
	Constraints Constraint
	Ref         string // FK: target table name. Empty = no FK.
	RefColumn   string // FK: target column. Empty = auto-detect PK of Ref table.
}

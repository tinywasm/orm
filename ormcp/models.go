package ormcp

// orm:no_db
type QueryArgs struct {
	SQL string `json:"SQL" db:"not_null"`
}

// orm:no_db
type ExecArgs struct {
	SQL string `json:"SQL" db:"not_null"`
}

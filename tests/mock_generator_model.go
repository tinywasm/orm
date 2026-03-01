package tests

import "time"

//go:generate ormgen -struct User

type User struct {
	ID        int     `db:"pk"`
	FirstName string  `db:"not_null"`
	LastName  string
	Email     string  `db:"unique"`
	Score     float64
	IsActive  bool
	Avatar    []byte
	age       int // unexported
}

type Order struct {
	ID        string `db:"pk"`
	UserID    int    `db:"ref=users:id"`
	Total     float64
}

type BadTime struct {
	CreatedAt time.Time
}

type BadAutoInc struct {
	ID string `db:"autoincrement"`
}

type Unsupp struct {
	Ch chan int
}

// NumericTypes covers int32, uint64, float32 mapping and bitmask constraints.
type NumericTypes struct {
	IDNumeric int32   `db:"pk,not_null"` // PK + NotNull → bitmask 5
	CountUint uint64
	RatioF32  float32
}

// RefNoColumn covers db:"ref=table" without a specific column (RefColumn must be "").
type RefNoColumn struct {
	IDRef    string `db:"pk"`
	ParentID int64  `db:"ref=parents"`
}

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

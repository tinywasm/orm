package tests

import (
	"time"

	"github.com/tinywasm/fmt"
)

//go:generate ormc

// orm:typed_fields
type User struct {
	ID        int    `db:"pk"`
	FirstName string `db:"not_null"`
	LastName  string
	Email     string `db:"unique"`
	Score     float64
	IsActive  bool
	Avatar    []byte
	age       int // unexported
}

type Order struct {
	ID     string `db:"pk"`
	UserID int    `db:"ref=user:id"`
	Total  float64
}

type BadTimeNoTag struct {
	ID        string `db:"pk"`
	Name      string
	CreatedAt time.Time
}

type ModelWithIgnored struct {
	ID      string `db:"pk"`
	Name    string
	Tags    []string `db:"-"` // slice: silently ignored
	Friends []User   `db:"-"` // struct slice: silently ignored
	Score   float64
}

type MultiA struct {
	ID   string `db:"pk"`
	Name string
}

func (MultiA) ModelName() string { return "multi_a_records" } // manually declared → D5

type MultiB struct {
	ID    string `db:"pk"`
	Value int64
}

type BadAutoInc struct {
	ID string `db:"autoincrement"`
}

type Unsupp struct {
	Ch chan int
}

// NumericTypes covers int32, uint64, float32 mapping and bitmask constraints.
type NumericTypes struct {
	IDNumeric int32 `db:"pk,not_null"` // PK + NotNull → bitmask 5
	CountUint uint64
	RatioF32  float32
}

// RefNoColumn covers db:"ref=table" without a specific column (RefColumn must be "").
type RefNoColumn struct {
	IDRef    string `db:"pk"`
	ParentID int64  `db:"ref=parent"`
}

// PointerReceiver tests that detectTableName handles pointer receivers (*T).
type PointerReceiver struct {
	ID   string `db:"pk"`
	Name string
}

func (*PointerReceiver) ModelName() string { return "ptr_table" }

// MockParent / MockChild: relation auto-detection fixture.
type MockParent struct {
	ID   string
	Name string
	Kids []MockChild // no tag — relation auto-detected via MockChild.MockParentID
}

type MockChild struct {
	ID           string `db:"pk"`
	MockParentID string `db:"ref=mock_parent"`
	Value        string
}

// orm:form_widgets
type UserForm struct {
	ID       string `db:"pk"`
	Name     string `input:"name,min=2,max=100"`
	Email    string `db:"not_null" input:"email,required"`
	Password string `input:"password,required,min=8"`
	Bio      string `input:"textarea,tilde,spaces"`
	Age      int64
}

// orm:form_widgets
// orm:no_db
type LoginForm struct {
	Email    string `input:"email,required"`
	Password string `input:"password,required"`
}

// TimeSlotResponse is a pure transport struct: formonly, no db: tags, no input: tags.
// Represents the pattern used by MCP tools that return computed results (e.g. availability slots).
// It must generate a {Name}List for json.Encode slice support even though it has no DB layer.
// orm:no_db
type TimeSlotResponse struct {
	StartUTC int64
	EndUTC   int64
}

type Address struct {
	Street string
	City   string
}

// orm:form_widgets
type UserWithJSON struct {
	ID       string  `db:"pk"`
	Name     string  ``
	Email    string  `input:"email"`
	Bio      string  `input:"textarea"`
	HomeAddr Address ``
}

type WithPointers struct {
	ID    string   `db:"pk"`
	Count *int     // pointer to primitive -> should be skipped with warning
	Addr  *Address // pointer to struct -> FieldStruct
}

// orm:no_db
type MCPResponse struct {
	Result fmt.RawJSON
	Error  fmt.RawJSON `json:",omitempty"`
}

// orm:no_db
type PlainResponse struct {
	Message string `json:"message"`
	Code    string `json:",omitempty"`
}

// orm:form_widgets
type UserWithNoTilde struct {
	ID     string `db:"pk"`
	Nombre string `input:"required,notilde"`
}

// orm:form_widgets
type UserWithNoTildeAndMin struct {
	ID     string `db:"pk"`
	Nombre string `input:"required,min=2,notilde"`
}

// orm:form_widgets
type ShortAutoInc struct {
	ID    int `db:"pk,autoinc"`
	Value int64
}

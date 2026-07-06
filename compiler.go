package orm

import "github.com/tinywasm/model"


// Compiler converts ORM queries into engine instructions.
type Compiler interface {
	Compile(q Query, m model.Model) (Plan, error)
}

package orm

import "github.com/tinywasm/model"

import (
	"testing"
)

type mockCompiler struct{ Compiler }

func (m *mockCompiler) Compile(q Query, model model.Model) (Plan, error) {
	return Plan{Query: "MOCK"}, nil
}

func TestCompilerAccessor(t *testing.T) {
	compiler := &mockCompiler{}
	db := New(nil, compiler)

	if db.Compiler() != compiler {
		t.Errorf("Compiler() did not return the expected compiler")
	}
}

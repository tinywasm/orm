package orm

import (
	"testing"
)

type mockCompiler struct{ Compiler }

func TestModelRegistry_NoDuplicates(t *testing.T) {
	db := New(nil, nil)
	m := &schemaModel{name: "users"}

	db.registerModel(m)
	db.registerModel(m)

	models := db.RegisteredModels()
	if len(models) != 1 {
		t.Errorf("expected 1 model, got %d", len(models))
	}
}

func TestCompilerAccessor(t *testing.T) {
	compiler := &mockCompiler{}
	db := New(nil, compiler)

	if db.Compiler() != compiler {
		t.Errorf("Compiler() did not return the expected compiler")
	}
}

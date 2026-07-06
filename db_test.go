package orm

import "github.com/tinywasm/model"

import (
	"testing"
)

type mockCompiler struct{ Compiler }

func (m *mockCompiler) Compile(q Query, model model.Model) (Plan, error) {
	return Plan{Query: "MOCK"}, nil
}

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

type mockTxBoundExecutor struct {
	Executor
}

func (m *mockTxBoundExecutor) Exec(query string, args ...any) error { return nil }
func (m *mockTxBoundExecutor) Commit() error                        { return nil }
func (m *mockTxBoundExecutor) Rollback() error                      { return nil }

type mockTxExecutor struct {
	Executor
}

func (m *mockTxExecutor) Exec(query string, args ...any) error { return nil }
func (m *mockTxExecutor) BeginTx() (TxBoundExecutor, error) {
	return &mockTxBoundExecutor{}, nil
}

func TestSyncSchema_RegistersModel(t *testing.T) {
	db := New(&mockTxExecutor{}, &mockCompiler{})
	fields := []model.Field{{Name: "id", Type: model.FieldInt}}

	err := db.SyncSchema("logs", fields)
	if err != nil {
		t.Fatalf("SyncSchema failed: %v", err)
	}

	models := db.RegisteredModels()
	found := false
	for _, m := range models {
		if m.ModelName() == "logs" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected model 'logs' to be registered")
	}
}

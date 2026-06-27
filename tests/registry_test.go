package tests

import (
	"testing"

	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
)

func TestModelRegistry_NoDuplicates(t *testing.T) {
	mockExec := &MockExecutor{}
	mockCompiler := &MockCompiler{}
	db := orm.New(mockExec, mockCompiler)

	user := &MockModel{Table: "users"}

	// Sync twice
	if err := db.Sync(user); err != nil {
		t.Fatal(err)
	}
	if err := db.Sync(user); err != nil {
		t.Fatal(err)
	}

	models := db.RegisteredModels()
	if len(models) != 1 {
		t.Errorf("expected 1 registered model, got %d", len(models))
	}
	if models[0].ModelName() != "users" {
		t.Errorf("expected model name 'users', got %s", models[0].ModelName())
	}
}

func TestCompilerAccessor(t *testing.T) {
	mockExec := &MockExecutor{}
	mockCompiler := &MockCompiler{}
	db := orm.New(mockExec, mockCompiler)

	if db.Compiler() != mockCompiler {
		t.Error("db.Compiler() did not return the expected compiler")
	}
}

func TestSyncSchema_Registry(t *testing.T) {
	mockExec := &MockExecutor{}
	mockCompiler := &MockCompiler{}
	db := orm.New(mockExec, mockCompiler)

	fields := []fmt.Field{{Name: "id", Type: fmt.FieldInt}}
	if err := db.SyncSchema("logs", fields); err != nil {
		t.Fatal(err)
	}

	models := db.RegisteredModels()
	if len(models) != 1 {
		t.Fatalf("expected 1 registered model, got %d", len(models))
	}
	if models[0].ModelName() != "logs" {
		t.Errorf("expected model name 'logs', got %s", models[0].ModelName())
	}
}

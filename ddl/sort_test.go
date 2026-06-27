package ddl

import (
	"testing"

	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
)

type mockModel struct {
	name string
	exts []orm.FieldExt
}

func (m *mockModel) ModelName() string           { return m.name }
func (m *mockModel) Schema() []fmt.Field         { return nil }
func (m *mockModel) Pointers() []any             { return nil }
func (m *mockModel) IsNil() bool                 { return m == nil }
func (m *mockModel) EncodeFields(fmt.FieldWriter) {}
func (m *mockModel) DecodeFields(fmt.FieldReader) {}
func (m *mockModel) SchemaExt() []orm.FieldExt   { return m.exts }

func TestTopologicalSort_NoDeps(t *testing.T) {
	users := &mockModel{name: "users"}
	roles := &mockModel{name: "roles"}
	models := []fmt.Model{users, roles}

	sorted, err := TopologicalSort(models)
	if err != nil {
		t.Fatal(err)
	}
	if len(sorted) != 2 {
		t.Errorf("expected 2 models, got %d", len(sorted))
	}
}

func TestTopologicalSort_WithFK(t *testing.T) {
	users := &mockModel{name: "users"}
	sessions := &mockModel{
		name: "sessions",
		exts: []orm.FieldExt{{Ref: "users"}},
	}
	models := []fmt.Model{sessions, users}

	sorted, err := TopologicalSort(models)
	if err != nil {
		t.Fatal(err)
	}

	userIndex, sessionIndex := -1, -1
	for i, m := range sorted {
		if m.ModelName() == "users" {
			userIndex = i
		} else if m.ModelName() == "sessions" {
			sessionIndex = i
		}
	}

	if userIndex > sessionIndex {
		t.Errorf("users should come before sessions (FK dependency)")
	}
}

func TestTopologicalSort_Cycle(t *testing.T) {
	a := &mockModel{name: "a", exts: []orm.FieldExt{{Ref: "b"}}}
	b := &mockModel{name: "b", exts: []orm.FieldExt{{Ref: "a"}}}
	models := []fmt.Model{a, b}

	_, err := TopologicalSort(models)
	if err == nil {
		t.Fatal("expected error on circular dependency")
	}
	if !fmt.Contains(err.Error(), "circular") {
		t.Errorf("expected circular dependency error, got %v", err)
	}
}

type noExtModel struct {
	name string
}

func (m *noExtModel) ModelName() string           { return m.name }
func (m *noExtModel) Schema() []fmt.Field         { return nil }
func (m *noExtModel) Pointers() []any             { return nil }
func (m *noExtModel) IsNil() bool                 { return m == nil }
func (m *noExtModel) EncodeFields(fmt.FieldWriter) {}
func (m *noExtModel) DecodeFields(fmt.FieldReader) {}

func TestTopologicalSort_NoSchemaExt(t *testing.T) {
	m := &noExtModel{name: "simple"}
	sorted, err := TopologicalSort([]fmt.Model{m})
	if err != nil {
		t.Fatal(err)
	}
	if len(sorted) != 1 || sorted[0].ModelName() != "simple" {
		t.Error("failed to handle model without SchemaExt")
	}
}

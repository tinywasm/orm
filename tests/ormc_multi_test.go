//go:build !wasm

package tests

import (
	"os"
	"strings"
	"testing"

	"github.com/tinywasm/orm"
)

func TestOrmc_MultiStruct(t *testing.T) {
	t.Run("Both structs appear in a single output file", func(t *testing.T) {
		o := orm.NewOrmc()
		infoA, err := o.ParseStruct("MultiA", "mock_generator_model.go")
		if err != nil {
			t.Fatal(err)
		}

		infoB, err := o.ParseStruct("MultiB", "mock_generator_model.go")
		if err != nil {
			t.Fatal(err)
		}

		err = o.GenerateForFile([]orm.StructInfo{infoA, infoB}, "mock_generator_model.go")
		if err != nil {
			t.Fatal(err)
		}

		outFile := "mock_generator_model_orm.go"
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(outFile)

		s := string(content)

		// Both schemas must be present
		if !strings.Contains(s, "func (m *MultiA) Schema()") {
			t.Error("MultiA Schema() not generated")
		}
		if !strings.Contains(s, "func (m *MultiB) Schema()") {
			t.Error("MultiB Schema() not generated")
		}
	})
}

func TestOrmc_TableNameDetection(t *testing.T) {
	t.Run("TableName() NOT generated when already declared (D5)", func(t *testing.T) {
		err := orm.NewOrmc().GenerateForStruct("MultiA", "mock_generator_model.go")
		if err != nil {
			t.Fatal(err)
		}

		outFile := "mock_generator_model_orm.go"
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(outFile)

		if strings.Contains(string(content), "func (m *MultiA) TableName()") {
			t.Error("TableName() must NOT be generated — already declared in source")
		}
	})

	t.Run("TableName() IS generated when not declared (D5)", func(t *testing.T) {
		err := orm.NewOrmc().GenerateForStruct("MultiB", "mock_generator_model.go")
		if err != nil {
			t.Fatal(err)
		}

		outFile := "mock_generator_model_orm.go"
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(outFile)

		if !strings.Contains(string(content), "func (m *MultiB) TableName()") {
			t.Error("TableName() must be generated — not declared in source")
		}
	})
}

func TestOrmc_DbIgnoreTag(t *testing.T) {
	t.Run("db:\"-\" fields excluded from Schema, Values, Pointers (D3+D4)", func(t *testing.T) {
		err := orm.NewOrmc().GenerateForStruct("ModelWithIgnored", "mock_generator_model.go")
		if err != nil {
			t.Fatal(err)
		}

		outFile := "mock_generator_model_orm.go"
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(outFile)

		s := string(content)

		for _, absent := range []string{"Tags", "Friends", "tags", "friends"} {
			if strings.Contains(s, absent) {
				t.Errorf("db:\"-\" field %q must be absent from ALL generated code", absent)
			}
		}
		for _, present := range []string{
			`"id"`, `"name"`, `"score"`,
			"m.ID", "m.Name", "m.Score",
			"&m.ID", "&m.Name", "&m.Score",
		} {
			if !strings.Contains(s, present) {
				t.Errorf("Non-ignored field %q must be present", present)
			}
		}
	})
}

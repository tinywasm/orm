package ormc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tinywasm/fmt"
)

// writeTemp writes content to a temp file and returns its path.
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "model.go")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestParseStruct_ValueStructField checks Bug 1: field of value struct type (not pointer)
// must produce IsPointer=false so generate.go emits &m.Field instead of nil checks.
func TestParseStruct_ValueStructField(t *testing.T) {
	src := `package p
// ormc:formonly
type Parent struct {
	Name  string
	Child Child
}
// ormc:formonly
type Child struct { X string }
`
	g := New()
	info, err := g.ParseStruct("Parent", writeTemp(t, src))
	if err != nil {
		t.Fatal(err)
	}
	var childField *FieldInfo
	for i := range info.Fields {
		if info.Fields[i].Name == "Child" {
			childField = &info.Fields[i]
		}
	}
	if childField == nil {
		t.Fatal("field Child not found")
	}
	if childField.Type != fmt.FieldStruct {
		t.Fatalf("expected FieldStruct, got %v", childField.Type)
	}
	if childField.IsPointer {
		t.Fatal("IsPointer should be false for value field")
	}
}

// TestParseStruct_PointerStructField checks that pointer struct fields keep IsPointer=true.
func TestParseStruct_PointerStructField(t *testing.T) {
	src := `package p
// ormc:formonly
type Parent struct {
	Name  string
	Child *Child
}
// ormc:formonly
type Child struct { X string }
`
	g := New()
	info, err := g.ParseStruct("Parent", writeTemp(t, src))
	if err != nil {
		t.Fatal(err)
	}
	var childField *FieldInfo
	for i := range info.Fields {
		if info.Fields[i].Name == "Child" {
			childField = &info.Fields[i]
		}
	}
	if childField == nil {
		t.Fatal("field Child not found")
	}
	if !childField.IsPointer {
		t.Fatal("IsPointer should be true for pointer field")
	}
}

// TestParseStruct_TypeAlias checks Bug 2: type alias for string must produce FieldText.
func TestParseStruct_TypeAlias(t *testing.T) {
	src := `package p
type MyID = string
// ormc:formonly
type Model struct {
	ID   MyID
	Name string
}
`
	g := New()
	info, err := g.ParseStruct("Model", writeTemp(t, src))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range info.Fields {
		if f.Name == "ID" {
			if f.Type != fmt.FieldText {
				t.Fatalf("ID: expected FieldText (alias of string), got %v", f.Type)
			}
			return
		}
	}
	t.Fatal("field ID not found")
}

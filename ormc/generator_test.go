package ormc

import (
	"os"
	"path/filepath"
	"strings"
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
type Parent struct {
	ID    int ` + "`" + `db:"pk"` + "`" + `
	Name  string
	Child Child
}
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
type Parent struct {
	ID    int ` + "`" + `db:"pk"` + "`" + `
	Name  string
	Child *Child
}
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
type Model struct {
	ID   MyID ` + "`" + `db:"pk"` + "`" + `
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

func TestGenerate_E2E(t *testing.T) {
	src := `package p
type MyID = string
type MyInt = int

type Parent struct {
	ID    MyID  ` + "`" + `db:"pk"` + "`" + `
	Count MyInt
	Child Child
}

type Child struct {
	X string
}
`
	tmpFile := writeTemp(t, src)
	g := New()
	infos, err := g.parseStructsInFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	err = g.GenerateForFile(infos, tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	genFile := strings.TrimSuffix(tmpFile, ".go") + "_orm.go"
	content, err := os.ReadFile(genFile)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)

	// Bug 1 Verification: struct by value should use &m.Field and NO nil check
	if !strings.Contains(s, "w.Object(\"child\", &m.Child)") {
		t.Errorf("missing expected w.Object for value struct field in EncodeFields")
	}
	if strings.Contains(s, "if m.Child != nil") {
		t.Errorf("unexpected nil check for value struct field in EncodeFields")
	}
	if !strings.Contains(s, "r.Object(\"child\", &m.Child)") {
		t.Errorf("missing expected r.Object for value struct field in DecodeFields")
	}

	// Bug 2 Verification: type aliases should map to primitive field types
	if !strings.Contains(s, "{Name: \"id\", Type: fmt.FieldText") {
		t.Errorf("MyID (string alias) should map to FieldText")
	}
	if !strings.Contains(s, "{Name: \"count\", Type: fmt.FieldInt") {
		t.Errorf("MyInt (int alias) should map to FieldInt")
	}
}

func TestParseStruct_SliceOfTypeAlias(t *testing.T) {
	src := `package p
type MyInt = int
type Model struct {
	ID  int ` + "`" + `db:"pk"` + "`" + `
	IDs []MyInt
}
`
	g := New()
	info, err := g.ParseStruct("Model", writeTemp(t, src))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range info.Fields {
		if f.Name == "IDs" {
			if f.Type != fmt.FieldIntSlice {
				t.Fatalf("IDs: expected FieldIntSlice (slice of alias of int), got %v", f.Type)
			}
			return
		}
	}
	t.Fatal("field IDs not found")
}

func TestGenerate_RawField(t *testing.T) {
	src := `package p
type Model struct {
	ID     int ` + "`" + `db:"pk"` + "`" + `
	Config string ` + "`" + `json:"raw"` + "`" + `
}
`
	tmpFile := writeTemp(t, src)
	g := New()
	infos, err := g.parseStructsInFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	err = g.GenerateForFile(infos, tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	genFile := strings.TrimSuffix(tmpFile, ".go") + "_orm.go"
	content, err := os.ReadFile(genFile)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)

	if !strings.Contains(s, "{Name: \"config\", Type: fmt.FieldRaw") {
		t.Errorf("config field should map to FieldRaw")
	}

	// Verify EncodeFields uses w.Raw()
	if !strings.Contains(s, "w.Raw(\"config\", m.Config)") {
		t.Errorf("missing expected w.Raw for FieldRaw in EncodeFields")
	}
	if strings.Contains(s, "w.String(\"config\", m.Config)") {
		t.Errorf("unexpected w.String for FieldRaw in EncodeFields")
	}

	// Verify DecodeFields uses r.Raw()
	if !strings.Contains(s, "if v, ok := r.Raw(\"config\"); ok { m.Config = v }") {
		t.Errorf("missing expected r.Raw for FieldRaw in DecodeFields")
	}
	if strings.Contains(s, "r.String(\"config\")") {
		t.Errorf("unexpected r.String for FieldRaw in DecodeFields")
	}
}

func TestGenerate_OmitEmpty(t *testing.T) {
	src := `package p
type Model struct {
	ID    int    ` + "`" + `db:"pk"` + "`" + `
	Text  string ` + "`" + `omitempty:"true"` + "`" + `
	Raw   string ` + "`" + `json:"raw,omitempty"` + "`" + `
	Int   int    ` + "`" + `json:",omitempty"` + "`" + `
	Bool  bool   ` + "`" + `omitempty:"true"` + "`" + `
	Child *Child ` + "`" + `omitempty:"true"` + "`" + `
	Plain string
}
type Child struct { X string }
`
	tmpFile := writeTemp(t, src)
	g := New()
	infos, err := g.parseStructsInFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	err = g.GenerateForFile(infos, tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	genFile := strings.TrimSuffix(tmpFile, ".go") + "_orm.go"
	content, err := os.ReadFile(genFile)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)

	// Verify schema has OmitEmpty: true
	if !strings.Contains(s, "{Name: \"text\", Type: fmt.FieldText, OmitEmpty: true}") {
		t.Errorf("text field should have OmitEmpty: true in schema")
	}
	if !strings.Contains(s, "{Name: \"raw\", Type: fmt.FieldRaw, OmitEmpty: true}") {
		t.Errorf("raw field should have OmitEmpty: true in schema")
	}

	// Verify EncodeFields has guards
	if !strings.Contains(s, "if m.Text != \"\" { w.String(\"text\", m.Text) }") {
		t.Errorf("missing guard for Text")
	}
	if !strings.Contains(s, "if len(m.Raw) != 0 { w.Raw(\"raw\", m.Raw) }") {
		t.Errorf("missing guard for Raw")
	}
	if !strings.Contains(s, "if m.Int != 0 { w.Int(\"int\", int64(m.Int)) }") {
		t.Errorf("missing guard for Int")
	}
	if !strings.Contains(s, "if m.Bool { w.Bool(\"bool\", m.Bool) }") {
		t.Errorf("missing guard for Bool")
	}
	if !strings.Contains(s, "if m.Child != nil { w.Object(\"child\", m.Child) }") {
		t.Errorf("missing guard for Child")
	}

	// Verify Plain field does NOT have a guard
	if !strings.Contains(s, "\tw.String(\"plain\", m.Plain)") {
		t.Errorf("Plain field should not have a guard")
	}
}

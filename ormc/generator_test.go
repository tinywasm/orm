package ormc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
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

func TestOnDelete_Default(t *testing.T) {
	src := `package p
type Session struct {
    UserID int64 ` + "`" + `db:"ref=users"` + "`" + `
}
`
	g := New()
	info, err := g.ParseStruct("Session", writeTemp(t, src))
	if err != nil {
		t.Fatal(err)
	}
	if info.Fields[0].OnDelete != "" {
		t.Errorf("expected empty OnDelete (defaults to CASCADE), got %q", info.Fields[0].OnDelete)
	}
}

func TestOnDelete_Restrict(t *testing.T) {
	src := `package p
type Session struct {
    UserID int64 ` + "`" + `db:"ref=users,on_delete=restrict"` + "`" + `
}
`
	g := New()
	info, err := g.ParseStruct("Session", writeTemp(t, src))
	if err != nil {
		t.Fatal(err)
	}
	if info.Fields[0].OnDelete != "restrict" {
		t.Errorf("expected OnDelete restrict, got %q", info.Fields[0].OnDelete)
	}
}

func TestOnDelete_Invalid(t *testing.T) {
	src := `package p
type Session struct {
    UserID int64 ` + "`" + `db:"ref=users,on_delete=wipe"` + "`" + `
}
`
	g := New()
	_, err := g.ParseStruct("Session", writeTemp(t, src))
	if err == nil {
		t.Fatal("expected error for invalid on_delete value")
	}
	if !fmt.Contains(err.Error(), "must be cascade|set_null|restrict|no_action") {
		t.Errorf("expected validation error message, got %v", err)
	}
}

func TestGenerate_SchemaExt(t *testing.T) {
	src := `package p
type Session struct {
	ID     int64 ` + "`" + `db:"pk"` + "`" + `
    UserID int64 ` + "`" + `db:"ref=users,on_delete=restrict"` + "`" + `
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

	if !strings.Contains(s, "func (m *Session) SchemaExt() []orm.FieldExt {") {
		t.Error("missing SchemaExt implementation")
	}
	if !strings.Contains(s, "{Field: _schemaSession[1], Ref: \"users\", OnDelete: \"restrict\"}") {
		t.Errorf("missing expected SchemaExt entry, got:\n%s", s)
	}
}

type mockExporter struct {
	models []fmt.Model
}

func (m *mockExporter) ExportDDL(models []fmt.Model) (string, error) {
	m.models = models
	return "CREATE TABLE mock", nil
}

func TestExportSQL_TwoTablesWithFK(t *testing.T) {
	src := `package p
type User struct {
    ID int ` + "`" + `db:"pk"` + "`" + `
}
type Session struct {
    ID     int ` + "`" + `db:"pk"` + "`" + `
    UserID int ` + "`" + `db:"ref=users,on_delete=cascade"` + "`" + `
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "model.go")
	os.WriteFile(path, []byte(src), 0644)

	g := New()
	exporter := &mockExporter{}
	sql, err := g.ExportSQL(dir, exporter)
	if err != nil {
		t.Fatal(err)
	}
	if sql != "CREATE TABLE mock" {
		t.Errorf("expected mock SQL, got %q", sql)
	}
	if len(exporter.models) != 2 {
		t.Errorf("expected 2 models passed to exporter, got %d", len(exporter.models))
	}

	// Check if stub has FK
	var session fmt.Model
	for _, m := range exporter.models {
		if m.ModelName() == "session" {
			session = m
		}
	}
	if session == nil {
		t.Fatal("session model stub not found")
	}
	ext, ok := session.(interface{ SchemaExt() []orm.FieldExt })
	if !ok {
		t.Fatal("session stub should implement SchemaExt")
	}
	exts := ext.SchemaExt()
	if len(exts) != 1 || exts[0].Ref != "users" || exts[0].OnDelete != "cascade" {
		t.Errorf("unexpected SchemaExt: %+v", exts)
	}
}

func TestModelStub_FieldTypes(t *testing.T) {
	info := StructInfo{
		ModelName: "test",
		Fields: []FieldInfo{
			{ColumnName: "f1", GoType: "int", NotNull: true},
			{ColumnName: "f2", GoType: "float64"},
			{ColumnName: "f3", GoType: "bool"},
			{ColumnName: "f4", GoType: "[]byte"},
			{ColumnName: "f5", GoType: "string", Maximum: 100},
		},
	}
	stub := newModelStub(info)
	schema := stub.Schema()
	if len(schema) != 5 {
		t.Fatalf("expected 5 fields, got %d", len(schema))
	}
	if schema[0].Type != fmt.FieldInt || !schema[0].NotNull {
		t.Error("f1 mismatch")
	}
	if schema[1].Type != fmt.FieldFloat {
		t.Error("f2 mismatch")
	}
	if schema[2].Type != fmt.FieldBool {
		t.Error("f3 mismatch")
	}
	if schema[3].Type != fmt.FieldBlob {
		t.Error("f4 mismatch")
	}
	if schema[4].Type != fmt.FieldText || schema[4].Permitted.Maximum != 100 {
		t.Errorf("f5 mismatch: type=%v, max=%d", schema[4].Type, schema[4].Permitted.Maximum)
	}
}

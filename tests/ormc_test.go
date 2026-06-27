//go:build !wasm

package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinywasm/orm/ormc"
)

func TestOrmc(t *testing.T) {
	// Regression: no_db structs with only primitive fields (no db: / input: tags)
	// must still generate {Name}List for fmt.FielderSlice support.
	//
	// This was broken: {Name}List was guarded by !info.NoDB, so transport-only
	// structs (e.g. TimeSlot returned by MCP list tools) never received a List type
	// and could not be encoded with json.Encode as a root array.
	//
	// Fix: {Name}List generation is unconditional — it is a transport concern, not a DB concern.
	t.Run("no_db pure transport struct generates List", func(t *testing.T) {
		err := ormc.New().GenerateForStruct("TimeSlotResponse", "models.go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		outFile := "models_orm.go"
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read generated file: %v", err)
		}
		defer os.Remove(outFile)

		s := string(content)

		// MUST generate Schema/Pointers (Fielder contract)
		mustHave := []string{
			"func (m *TimeSlotResponse) ModelName() string {",
			"func (m *TimeSlotResponse) Schema() []fmt.Field {",
			"func (m *TimeSlotResponse) Pointers() []any {",
			// MUST generate List with all five FielderSlice methods
			"type TimeSlotResponseList []*TimeSlotResponse",
			"func (s *TimeSlotResponseList) Schema() []fmt.Field { return nil }",
			"func (s *TimeSlotResponseList) Pointers() []any     { return nil }",
			"func (s *TimeSlotResponseList) Len() int",
			"func (s *TimeSlotResponseList) At(i int) fmt.Fielder",
			"func (s *TimeSlotResponseList) Append() fmt.Fielder",
		}
		for _, want := range mustHave {
			if !strings.Contains(s, want) {
				t.Errorf("missing: %s", want)
			}
		}

		// MUST NOT generate DB helpers — no DB layer for no_db
		mustNotHave := []string{
			"func ReadOneTimeSlotResponse",
			"func ReadAllTimeSlotResponse",
			"var TimeSlotResponse_ =",
			`"github.com/tinywasm/orm"
	"github.com/tinywasm/orm/ormc"`,
		}
		for _, bad := range mustNotHave {
			if strings.Contains(s, bad) {
				t.Errorf("must not contain: %s", bad)
			}
		}
	})

	t.Run("form_widgets + no_db directive", func(t *testing.T) {
		err := ormc.New().GenerateForStruct("LoginForm", "models.go")
		if err != nil {
			t.Fatalf("Failed to generate code for LoginForm: %v", err)
		}

		outFile := "models_orm.go"
		contentBytes, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("Failed to read generated file: %v", err)
		}
		defer os.Remove(outFile)

		content := string(contentBytes)

		// MUST generate Fielder methods
		expectedStrings := []string{
			"func (m *LoginForm) ModelName() string {",
			"func (m *LoginForm) Schema() []fmt.Field {",
			"func (m *LoginForm) Pointers() []any {",
			"Widget: input.Email()",
			"Widget: input.Password()",
			"\"github.com/tinywasm/form/input\"",
		}
		for _, expected := range expectedStrings {
			if !strings.Contains(content, expected) {
				t.Errorf("Generated file missing expected string: %s", expected)
			}
		}

		// MUST NOT generate ORM helpers
		forbiddenStrings := []string{
			"func (m *LoginForm) FormName() string",
			"func ReadOneLoginForm",
			"func ReadAllLoginForm",
			"var LoginForm_ =",
			"\"github.com/tinywasm/orm\"", // Import should be missing
		}
		// MUST generate List type for json.Encode support (no_db structs are also list-encodable)
		if !strings.Contains(content, "type LoginFormList []*LoginForm") {
			t.Error("no_db struct must generate LoginFormList for FielderSlice support")
		}
		for _, forbidden := range forbiddenStrings {
			if strings.Contains(content, forbidden) {
				t.Errorf("Generated file contains forbidden string: %s", forbidden)
			}
		}
	})

	t.Run("Validate tags and Permitted", func(t *testing.T) {
		err := ormc.New().GenerateForStruct("UserForm", "models.go")
		if err != nil {
			t.Fatalf("Failed to generate code for UserForm: %v", err)
		}

		outFile := "models_orm.go"
		contentBytes, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("Failed to read generated file: %v", err)
		}
		defer os.Remove(outFile)

		content := string(contentBytes)

		expectedStrings := []string{
			"func (m *UserForm) ModelName() string {",
			"return \"user_form\"",
			"{Name: \"name\", Type: fmt.FieldText, Widget: input.Text(), Permitted: fmt.Permitted{Letters: true, Tilde: true, Spaces: true, Minimum: 2, Maximum: 100}}",
			"{Name: \"email\", Type: fmt.FieldText, NotNull: true, Widget: input.Email()}",
			"{Name: \"password\", Type: fmt.FieldText, NotNull: true, Widget: input.Password(), Permitted: fmt.Permitted{Minimum: 8}}",
			"{Name: \"bio\", Type: fmt.FieldText, Widget: input.Textarea(), Permitted: fmt.Permitted{Tilde: true, Spaces: true}}",
			"{Name: \"id\", Type: fmt.FieldText, DB: &fmt.FieldDB{PK: true}, Widget: input.Text()}",
			"func (m *UserForm) Validate(action byte) error {",
			"return fmt.ValidateFields(action, m)",
			"\"github.com/tinywasm/form/input\"",
		}

		for _, expected := range expectedStrings {
			if !strings.Contains(content, expected) {
				t.Errorf("Generated file missing expected string: %s\nContent:\n%s", expected, content)
			}
		}
	})

	t.Run("Generate User", func(t *testing.T) {
		o := ormc.New()
		err := o.GenerateForStruct("User", "models.go")
		if err != nil {
			t.Fatalf("Failed to generate code for User: %v", err)
		}

		outFile := "models_orm.go"
		contentBytes, err := os.ReadFile(outFile)
		if err != nil {
			// File may not be created if no fields are mappable
			if os.IsNotExist(err) {
				return
			}
			t.Fatalf("Failed to read generated file: %v", err)
		}
		defer os.Remove(outFile)

		content := string(contentBytes)

		expectedStrings := []string{
			"// DO NOT EDIT. generated by github.com/tinywasm/orm",
			"package tests",
			"func (m *User) ModelName() string {",
			"return \"user\"",
			"func (m *User) Schema() []fmt.Field {",
			"{Name: \"id\", Type: fmt.FieldInt, DB: &fmt.FieldDB{PK: true}}",
			"{Name: \"first_name\", Type: fmt.FieldText, NotNull: true}",
			"{Name: \"last_name\", Type: fmt.FieldText},",
			"{Name: \"email\", Type: fmt.FieldText, DB: &fmt.FieldDB{Unique: true}}",
			"{Name: \"score\", Type: fmt.FieldFloat},",
			"{Name: \"is_active\", Type: fmt.FieldBool},",
			"{Name: \"avatar\", Type: fmt.FieldBlob},",
			"func (m *User) Pointers() []any {",
			"&m.ID",
			"&m.FirstName",
			"&m.LastName",
			"var User_ = struct {",
			"ID: \"id\"",
			"func ReadOneUser(qb *orm.QB, model *User) (*User, error) {",
			"func ReadAllUser(qb *orm.QB) (UserList, error) {",
			"type UserList []*User",
			"func (s *UserList) Schema() []fmt.Field { return nil }",
			"func (s *UserList) Pointers() []any     { return nil }",
			"func (s *UserList) Len() int             { return len(*s) }",
			"func (s *UserList) At(i int) fmt.Fielder { return (*s)[i] }",
			"func (s *UserList) Append() fmt.Fielder",
			"func (m *User) EncodeFields(w fmt.FieldWriter) {",
			"func (m *User) DecodeFields(r fmt.FieldReader) {",
			"func (m *User) IsNil() bool {",
		}

		for _, expected := range expectedStrings {
			if !strings.Contains(content, expected) {
				t.Errorf("Generated file missing expected string: %s", expected)
			}
		}

		if strings.Contains(content, "\"age\"") || strings.Contains(content, "m.age") || strings.Contains(content, "&m.age") || strings.Contains(content, "Age:") {
			t.Errorf("Generated file should not contain unexported field 'age'")
		}
	})

	t.Run("Generate Order With Refs", func(t *testing.T) {
		err := ormc.New().GenerateForStruct("Order", "models.go")
		if err != nil {
			t.Fatalf("Failed to generate code for Order: %v", err)
		}

		outFile := "models_orm.go"
		contentBytes, err := os.ReadFile(outFile)
		if err != nil {
			if os.IsNotExist(err) {
				return
			}
			t.Fatalf("Failed to read generated file: %v", err)
		}
		defer os.Remove(outFile)

		content := string(contentBytes)

		expectedStrings := []string{
			"{Name: \"id\", Type: fmt.FieldText, DB: &fmt.FieldDB{PK: true}}",
			"{Name: \"user_id\", Type: fmt.FieldInt},",
		}

		for _, expected := range expectedStrings {
			if !strings.Contains(content, expected) {
				t.Errorf("Generated file missing expected string: %s", expected)
			}
		}
	})

	t.Run("Bad Time Type — now a warning, not fatal", func(t *testing.T) {
		// D8: time.Time without db:"-" → warning + skip, not error
		err := ormc.New().GenerateForStruct("BadTimeNoTag", "models.go")
		if err != nil {
			t.Fatalf("Expected no error for time.Time (warn+skip), got: %v", err)
		}

		outFile := "models_orm.go"
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("Failed to read generated file: %v", err)
		}
		defer os.Remove(outFile)

		s := string(content)
		// CreatedAt must be absent (skipped)
		if strings.Contains(s, "CreatedAt") || strings.Contains(s, "created_at") {
			t.Error("time.Time field must be absent from generated output")
		}
		// ID and Name must be present
		if !strings.Contains(s, `"id"`) || !strings.Contains(s, `"name"`) {
			t.Error("Other fields must still be generated")
		}
	})

	t.Run("Bad AutoInc", func(t *testing.T) {
		err := ormc.New().GenerateForStruct("BadAutoInc", "models.go")
		if err == nil || !strings.Contains(err.Error(), "autoincrement not allowed on FieldText") {
			t.Errorf("Expected error about autoincrement on FieldText, got %v", err)
		}
	})

	t.Run("Unsupported Type", func(t *testing.T) {
		err := ormc.New().GenerateForStruct("Unsupp", "models.go")
		if err != nil {
			t.Fatalf("Did not expect error for unsupported type, got %v", err)
		}

		outFile := "models_orm.go"
		contentBytes, err := os.ReadFile(outFile)
		if err != nil {
			if os.IsNotExist(err) {
				return
			}
			t.Fatalf("Failed to read generated file: %v", err)
		}
		defer os.Remove(outFile)

		content := string(contentBytes)
		if strings.Contains(content, "Ch") {
			t.Errorf("Expected 'Ch' to be absent in output, but it was generated")
		}
	})

	t.Run("Numeric Type Mapping and Bitmask", func(t *testing.T) {
		err := ormc.New().GenerateForStruct("NumericTypes", "models.go")
		if err != nil {
			t.Fatalf("Failed to generate code for NumericTypes: %v", err)
		}

		outFile := "models_orm.go"
		contentBytes, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("Failed to read generated file: %v", err)
		}
		defer os.Remove(outFile)

		content := string(contentBytes)

		expectedStrings := []string{
			// int32 → FieldInt
			`{Name: "idnumeric", Type: fmt.FieldInt, DB: &fmt.FieldDB{PK: true}, NotNull: true}`,
			// uint64 → FieldInt
			`{Name: "count_uint", Type: fmt.FieldInt},`,
			// float32 → FieldFloat
			`{Name: "ratio_f32", Type: fmt.FieldFloat},`,
		}

		for _, expected := range expectedStrings {
			if !strings.Contains(content, expected) {
				t.Errorf("Generated file missing expected string: %s", expected)
			}
		}
	})

	t.Run("Ref Without Column", func(t *testing.T) {
		err := ormc.New().GenerateForStruct("RefNoColumn", "models.go")
		if err != nil {
			t.Fatalf("Failed to generate code for RefNoColumn: %v", err)
		}

		outFile := "models_orm.go"
		contentBytes, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("Failed to read generated file: %v", err)
		}
		defer os.Remove(outFile)

		content := string(contentBytes)

		// Ref should NOT be in Input anymore
		if strings.Contains(content, `Input: "ref=parent"`) {
			t.Errorf("Ref should NOT be in Input anymore, got:\n%s", content)
		}
	})

	t.Run("JSON tags and Nested structs", func(t *testing.T) {
		err := ormc.New().GenerateForStruct("UserWithJSON", "models.go")
		if err != nil {
			t.Fatalf("Failed to generate code for UserWithJSON: %v", err)
		}

		outFile := "models_orm.go"
		contentBytes, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("Failed to read generated file: %v", err)
		}
		defer os.Remove(outFile)

		content := string(contentBytes)

		expectedStrings := []string{
			`{Name: "id", Type: fmt.FieldText, DB: &fmt.FieldDB{PK: true}, Widget: input.Text()}`,
			`{Name: "name", Type: fmt.FieldText, Widget: input.Text()}`,
			`{Name: "email", Type: fmt.FieldText, Widget: input.Email()}`,
			`{Name: "bio", Type: fmt.FieldText, Widget: input.Textarea()}`,
			`{Name: "home_addr", Type: fmt.FieldStruct}`,
		}

		for _, expected := range expectedStrings {
			if !strings.Contains(content, expected) {
				t.Errorf("Generated file missing expected string: %s\nContent:\n%s", expected, content)
			}
		}
	})

	t.Run("Pointers to primitives vs structs", func(t *testing.T) {
		err := ormc.New().GenerateForStruct("WithPointers", "models.go")
		if err != nil {
			t.Fatalf("Failed to generate code for WithPointers: %v", err)
		}

		outFile := "models_orm.go"
		contentBytes, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("Failed to read generated file: %v", err)
		}
		defer os.Remove(outFile)

		content := string(contentBytes)

		// Addr (*Address) should be present as FieldStruct
		if !strings.Contains(content, `{Name: "addr", Type: fmt.FieldStruct}`) {
			t.Errorf("Generated file missing expected string for Addr:\n%s", content)
		}

		// Count (*int) should be ABSENT
		if !strings.Contains(content, `{Name: "id", Type: fmt.FieldText, DB: &fmt.FieldDB{PK: true}}`) {
			t.Errorf("Generated file missing expected string for ID:\n%s", content)
		}

		if strings.Contains(content, "Count") || strings.Contains(content, "count") {
			t.Errorf("Generated file should NOT contain 'count' field (pointer to primitive):\n%s", content)
		}
	})

	t.Run("FieldStruct for nested struct stage 1", func(t *testing.T) {
		err := ormc.New().GenerateForStruct("UserWithJSON", "models.go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		outFile := "models_orm.go"
		contentBytes, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read: %v", err)
		}
		defer os.Remove(outFile)
		content := string(contentBytes)
		if !strings.Contains(content, "fmt.FieldStruct") {
			t.Errorf("expected FieldStruct in generated output, got:\n%s", content)
		}
	})

	t.Run("fmt.RawJSON type generates FieldRaw", func(t *testing.T) {
		err := ormc.New().GenerateForStruct("MCPResponse", "models.go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		outFile := "models_orm.go"
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read generated file: %v", err)
		}
		defer os.Remove(outFile)

		s := string(content)

		mustHave := []string{
			`{Name: "result", Type: fmt.FieldRaw`,
			`{Name: "error", Type: fmt.FieldRaw, OmitEmpty: true`,
		}
		for _, want := range mustHave {
			if !strings.Contains(s, want) {
				t.Errorf("missing expected string: %s\nContent:\n%s", want, s)
			}
		}
	})

	t.Run("fields without raw stay FieldText", func(t *testing.T) {
		err := ormc.New().GenerateForStruct("PlainResponse", "models.go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		outFile := "models_orm.go"
		content, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("failed to read generated file: %v", err)
		}
		defer os.Remove(outFile)

		s := string(content)

		mustHave := []string{
			`{Name: "message", Type: fmt.FieldText`,
			`{Name: "code", Type: fmt.FieldText, OmitEmpty: true`,
		}
		for _, want := range mustHave {
			if !strings.Contains(s, want) {
				t.Errorf("missing expected string: %s\nContent:\n%s", want, s)
			}
		}
		if strings.Contains(s, "fmt.FieldRaw") {
			t.Error("should not contain FieldRaw")
		}
	})

	t.Run("ShortAutoInc with db autoinc tag", func(t *testing.T) {
		err := ormc.New().GenerateForStruct("ShortAutoInc", "models.go")
		if err != nil {
			t.Fatalf("Failed to generate code for ShortAutoInc: %v", err)
		}

		outFile := "models_orm.go"
		contentBytes, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("Failed to read generated file: %v", err)
		}
		defer os.Remove(outFile)

		content := string(contentBytes)
		expected := `{Name: "id", Type: fmt.FieldInt, DB: &fmt.FieldDB{PK: true, AutoInc: true}`
		if !strings.Contains(content, expected) {
			t.Errorf("Expected string: %q not found in output:\n%s", expected, content)
		}
	})
}

func TestParseStructRejectsJsonNameOnDBModel(t *testing.T) {
	// A struct with a db: tag is a DB struct. json:"name" on any field of a DB struct
	// is a compile error — column names are always derived as snake_case from the Go field name.
	src := `package test

type Product struct {
	ID    string ` + "`" + `db:"pk"` + "`" + `
	Price int64  ` + "`" + `db:"not_null" json:"unitPrice"` + "`" + `
}
`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "model.go")
	os.WriteFile(path, []byte(src), 0644)

	o := ormc.New()
	_, err := o.ParseStruct("Product", path)
	if err == nil {
		t.Fatal("expected error for json name override on DB struct, got nil")
	}
}

func TestParseStructFormOnlyAllowsJsonName(t *testing.T) {
	// Codec-only struct (no db: tags, no ID convention) — json name is valid and sets ColumnName.
	src := `package test

type ContactForm struct {
	FirstName string ` + "`" + `json:"firstName"` + "`" + `
}
`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "model.go")
	os.WriteFile(path, []byte(src), 0644)

	o := ormc.New()
	info, err := o.ParseStruct("ContactForm", path)
	if err != nil {
		t.Fatalf("unexpected error for formonly struct: %v", err)
	}
	if info.Fields[0].ColumnName != "firstName" {
		t.Errorf("expected ColumnName %q, got %q", "firstName", info.Fields[0].ColumnName)
	}
}

func TestGenerateWithoutFields(t *testing.T) {
	src := `package test

type User struct {
	ID   int    ` + "`" + `db:"pk"` + "`" + `
	Name string
}
`
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "model.go"), []byte(src), 0644)

	o := ormc.New()
	o.SetRootDir(tmpDir)
	if err := o.Run(); err != nil {
		t.Fatal(err)
	}
	output, _ := os.ReadFile(filepath.Join(tmpDir, "model_orm.go"))
	if strings.Contains(string(output), "var User_ =") {
		t.Error("field descriptor must not be generated without -fields flag")
	}
}

func TestGenerateWithFields(t *testing.T) {
	src := `package test

// orm:typed_fields
type User struct {
	ID   int    ` + "`" + `db:"pk"` + "`" + `
	Name string
}
`
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "model.go"), []byte(src), 0644)

	o := ormc.New()
	o.SetRootDir(tmpDir)
	if err := o.Run(); err != nil {
		t.Fatal(err)
	}
	output, _ := os.ReadFile(filepath.Join(tmpDir, "model_orm.go"))
	out := string(output)
	if !strings.Contains(out, "var User_ =") {
		t.Error("field descriptor must be generated with orm:typed_fields directive")
	}
	if !strings.Contains(out, `Name: "name"`) {
		t.Error("field descriptor must include field Name")
	}
}

func TestGenerateFormOnlyNeverFields(t *testing.T) {
	src := `package test

// orm:no_db
// orm:typed_fields
type ContactForm struct {
	Name string
}
`
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "model.go"), []byte(src), 0644)

	o := ormc.New()
	o.SetRootDir(tmpDir)
	if err := o.Run(); err != nil {
		t.Fatal(err)
	}
	output, _ := os.ReadFile(filepath.Join(tmpDir, "model_orm.go"))
	if strings.Contains(string(output), "var ContactForm_ =") {
		t.Error("field descriptor must never be generated for no_db structs")
	}
}

package ormc

import (
	"strings"
	"testing"
)

func TestRoleInference(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantForm bool
		wantDB   bool
	}{
		{
			name: "Form and DB",
			src: `package p
type Contact struct {
	ID      int    ` + "`" + `db:"pk,autoinc"` + "`" + `
	Name    string ` + "`" + `input:"text"` + "`" + `
	Message string ` + "`" + `input:"textarea"` + "`" + `
}
`,
			wantForm: true,
			wantDB:   true,
		},
		{
			name: "Form Only",
			src: `package p
type Search struct {
	Query string ` + "`" + `input:"text"` + "`" + `
}
`,
			wantForm: true,
			wantDB:   false,
		},
		{
			name: "DB Only by ID convention",
			src: `package p
type Log struct {
	ID      int
	Message string
}
`,
			wantForm: false,
			wantDB:   true,
		},
		{
			name: "DB Only by tag",
			src: `package p
type Internal struct {
	Key string ` + "`" + `db:"pk"` + "`" + `
	Val string
}
`,
			wantForm: false,
			wantDB:   true,
		},
		{
			name: "Codec Only",
			src: `package p
type Payload struct {
	Data string
}
`,
			wantForm: false,
			wantDB:   false,
		},
		{
			name: "Exclude from DB (Codec Only)",
			src: `package p
type Payload struct {
	ID   int ` + "`" + `db:"-"` + "`" + `
	Data string
}
`,
			wantForm: false,
			wantDB:   false,
		},
		{
			name: "Exclude from Form (DB Only)",
			src: `package p
type Secret struct {
	ID     int
	Secret string ` + "`" + `input:"-"` + "`" + `
}
`,
			wantForm: false,
			wantDB:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := New()
			tmpFile := writeTemp(t, tt.src)
			// Extract struct name from src
			lines := strings.Split(tt.src, "\n")
			structName := ""
			for _, line := range lines {
				if strings.HasPrefix(line, "type ") && strings.HasSuffix(line, " struct {") {
					structName = strings.TrimPrefix(line, "type ")
					structName = strings.TrimSuffix(structName, " struct {")
					break
				}
			}

			info, err := g.ParseStruct(structName, tmpFile)
			if err != nil {
				t.Fatalf("failed to parse struct: %v", err)
			}

			if info.IsForm != tt.wantForm {
				t.Errorf("IsForm = %v, want %v", info.IsForm, tt.wantForm)
			}
			if (info.NoDB) == tt.wantDB {
				t.Errorf("NoDB = %v, want %v (wantDB=%v)", info.NoDB, !tt.wantDB, tt.wantDB)
			}
		})
	}
}

func TestUnsupportedFields(t *testing.T) {
	src := `package p
type Bad struct {
	Handler func()
	Chan    chan int
}
type Good struct {
	Name string
}
`
	g := New()
	tmpFile := writeTemp(t, src)

	// Bad should be skipped (no mappable fields)
	info, err := g.ParseStruct("Bad", tmpFile)
	if err != nil {
		t.Fatalf("ParseStruct failed: %v", err)
	}
	if len(info.Fields) != 0 {
		t.Errorf("expected 0 fields for Bad, got %d", len(info.Fields))
	}

	infos, err := g.parseStructsInFile(tmpFile)
	if err != nil {
		t.Fatalf("parseStructsInFile failed: %v", err)
	}

	foundGood := false
	for _, info := range infos {
		if info.Name == "Bad" {
			t.Errorf("Bad struct should have been skipped")
		}
		if info.Name == "Good" {
			foundGood = true
		}
	}
	if !foundGood {
		t.Errorf("Good struct should have been found")
	}
}

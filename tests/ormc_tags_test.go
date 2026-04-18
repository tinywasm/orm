//go:build !wasm

package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinywasm/orm"
)

func TestRewriteModelTags(t *testing.T) {
	// Create a temp model file
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.go")

	input := `package test

type User struct {
	ID    int    ` + "`" + `db:"pk" json:"id"` + "`" + `
	Name  string ` + "`" + `json:"name" validate:"required"` + "`" + `
	Email string ` + "`" + `db:"unique" form:"email" json:"email,omitempty" validate:"email"` + "`" + `
	Age   int    ` + "`" + `json:"-"` + "`" + `
}
`
	err := os.WriteFile(modelPath, []byte(input), 0644)
	if err != nil {
		t.Fatal(err)
	}

	o := orm.NewOrmc()
	err = o.RewriteModelTags(modelPath)
	if err != nil {
		t.Fatalf("RewriteModelTags failed: %v", err)
	}

	output, err := os.ReadFile(modelPath)
	if err != nil {
		t.Fatal(err)
	}

	outStr := string(output)

	// Verify ID: json:"id" removed (DB struct — json name has no effect)
	if strings.Contains(outStr, `json:"id"`) {
		t.Errorf("Expected json:\"id\" to be removed, got: %s", outStr)
	}
	// Verify Name: json:"name" removed, validate:"required" removed
	if strings.Contains(outStr, `json:"name"`) || strings.Contains(outStr, `validate:"required"`) {
		t.Errorf("Expected json:\"name\" and validate:\"required\" to be removed, got: %s", outStr)
	}
	// Verify Email: form, validate removed, json rewritten to ,omitempty (discarded for DB model)
	if strings.Contains(outStr, `form:"email"`) || strings.Contains(outStr, `validate:"email"`) {
		t.Errorf("Expected form and validate to be removed, got: %s", outStr)
	}
	if strings.Contains(outStr, `json:",`) {
		t.Errorf("json tag with leading comma should not exist, got: %s", outStr)
	}
	// Verify Age: json:"-" preserved
	if !strings.Contains(outStr, `json:"-"`) {
		t.Errorf("Expected json:\"-\" to be preserved, got: %s", outStr)
	}
}

func TestRewriteModelTagsFormOnly(t *testing.T) {
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.go")

	input := `package test

// ormc:formonly
type Params struct {
	ProtocolVersion string ` + "`" + `json:"protocolVersion"` + "`" + `
	ClientInfo      string ` + "`" + `json:"clientInfo,omitempty" validate:"required"` + "`" + `
	Internal        string ` + "`" + `json:"-"` + "`" + `
}
`
	err := os.WriteFile(modelPath, []byte(input), 0644)
	if err != nil {
		t.Fatal(err)
	}

	o := orm.NewOrmc()
	if err := o.RewriteModelTags(modelPath); err != nil {
		t.Fatalf("RewriteModelTags failed: %v", err)
	}

	output, err := os.ReadFile(modelPath)
	if err != nil {
		t.Fatal(err)
	}
	outStr := string(output)

	// json name must be preserved for formonly
	if !strings.Contains(outStr, `json:"protocolVersion"`) {
		t.Errorf("json name must be preserved for formonly struct, got: %s", outStr)
	}
	// json name + omitempty must be preserved
	if !strings.Contains(outStr, `json:"clientInfo,omitempty"`) {
		t.Errorf("json name with omitempty must be preserved for formonly struct, got: %s", outStr)
	}
	// validate must be removed
	if strings.Contains(outStr, `validate:`) {
		t.Errorf("validate tag must be removed, got: %s", outStr)
	}
	// json:"-" must be preserved
	if !strings.Contains(outStr, `json:"-"`) {
		t.Errorf("json:\"-\" must be preserved, got: %s", outStr)
	}
}

func TestRewriteModelTagsFormOnlyLeadingComma(t *testing.T) {
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.go")

	input := `package test

// ormc:formonly
type Repro struct {
	Result string ` + "`" + `json:",omitempty,raw"` + "`" + `
	Error  string ` + "`" + `json:",omitempty,raw"` + "`" + `
}
`
	err := os.WriteFile(modelPath, []byte(input), 0644)
	if err != nil {
		t.Fatal(err)
	}

	o := orm.NewOrmc()
	if err := o.RewriteModelTags(modelPath); err != nil {
		t.Fatalf("RewriteModelTags failed: %v", err)
	}

	output, err := os.ReadFile(modelPath)
	if err != nil {
		t.Fatal(err)
	}
	outStr := string(output)

	if !strings.Contains(outStr, `json:",omitempty,raw"`) {
		t.Errorf("Expected leading comma to be preserved in json:\",omitempty,raw\", got: %s", outStr)
	}
}

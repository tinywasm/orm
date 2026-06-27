//go:build !wasm

package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinywasm/orm/ormc"
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

	o := ormc.New()
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

	// No struct-level directive — codec-only is inferred from field tags:
	// no db: tags, no ID field convention → json names must be preserved.
	input := `package test

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

	o := ormc.New()
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

func TestRewriteModelTagsFormOnlyStripsRaw(t *testing.T) {
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.go")

	// No struct-level directive — codec-only inferred from field tags.
	input := `package test

type Repro struct {
	Result string ` + "`" + `json:",omitempty,raw"` + "`" + `
	Error  string ` + "`" + `json:",omitempty,raw"` + "`" + `
}
`
	err := os.WriteFile(modelPath, []byte(input), 0644)
	if err != nil {
		t.Fatal(err)
	}

	o := ormc.New()
	if err := o.RewriteModelTags(modelPath); err != nil {
		t.Fatalf("RewriteModelTags failed: %v", err)
	}

	output, err := os.ReadFile(modelPath)
	if err != nil {
		t.Fatal(err)
	}
	outStr := string(output)

	// raw must be stripped from json tags (fields should use fmt.RawJSON type instead)
	if strings.Contains(outStr, `json:",omitempty,raw"`) || strings.Contains(outStr, `json:",raw"`) {
		t.Errorf("Expected raw to be stripped from json tags, got: %s", outStr)
	}
	// omitempty should be preserved
	if !strings.Contains(outStr, `json:",omitempty"`) {
		t.Errorf("Expected omitempty to be preserved, got: %s", outStr)
	}
}

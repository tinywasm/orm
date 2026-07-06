//go:build !wasm

package tests

import (
	"os"
	"strings"
	"testing"

	"github.com/tinywasm/orm/ormc"
)

func TestOrmc_Notilde(t *testing.T) {
	// 1. notilde is recognised as modifier (doesn't fail or warn as unknown widget)
	// 2. notilde emits input.SetTilde(<widget>, false)
	// 3. can be combined with other modifiers

	t.Run("notilde emits setter", func(t *testing.T) {
		err := ormc.New().GenerateForStruct("UserWithNoTilde", "models.go")
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

		// MUST emit .SetTilde(false)
		if !strings.Contains(s, "Widget: input.SetTilde(input.Text(), false)") {
			t.Errorf("expected .SetTilde(false) in Widget, got:\n%s", s)
		}
	})

	t.Run("notilde with other modifiers", func(t *testing.T) {
		err := ormc.New().GenerateForStruct("UserWithNoTildeAndMin", "models.go")
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

		// MUST emit .SetTilde(false) AND Permitted{Minimum: 2}
		if !strings.Contains(s, "Widget: input.SetTilde(input.Text(), false)") {
			t.Errorf("expected .SetTilde(false) in Widget, got:\n%s", s)
		}
		if !strings.Contains(s, "Permitted: model.Permitted{Minimum: 2}") {
			t.Errorf("expected Permitted{Minimum: 2}, got:\n%s", s)
		}
	})
}

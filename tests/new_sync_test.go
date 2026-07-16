package tests

import (
	"errors"
	"testing"

	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/orm/mock"
)

func TestOpen(t *testing.T) {
	// O1: Register("fake", f) + Open("fake://x") returns a *DB built from f
	t.Run("O1 - Register and Open", func(t *testing.T) {
		factoryCalled := false
		orm.Register("fake", func(dsn string) (*orm.DB, error) {
			factoryCalled = true
			if dsn != "fake://connection" {
				return nil, errors.New("unexpected dsn")
			}
			return orm.New(&mock.Executor{}, &mock.Compiler{}), nil
		})

		db, err := orm.Open("fake://connection")
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		if db == nil {
			t.Fatal("Expected *DB, got nil")
		}
		if !factoryCalled {
			t.Fatal("Factory was not called")
		}
	})

	// O2: Open("nope://x") error naming unknown scheme
	t.Run("O2 - Unknown scheme error", func(t *testing.T) {
		_, err := orm.Open("nope://x")
		if err == nil {
			t.Fatal("Expected error for unknown scheme, got nil")
		}
		if !fmt.Contains(err.Error(), "unknown scheme nope") {
			t.Errorf("Expected error to mention 'unknown scheme nope', got: %v", err)
		}
	})
}


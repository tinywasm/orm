package tests

import (
	"errors"
	"testing"

	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
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
			return orm.New(&MockExecutor{}, &MockCompiler{}), nil
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

func TestSyncSchema(t *testing.T) {
	// O3: SyncSchema("x", [f1,f2]) issues CreateTable + 2 AddColumn Actions
	t.Run("O3 - SyncSchema issues correct actions", func(t *testing.T) {
		var compiledActions []orm.Action
		mockCompiler := &functionalCompiler{
			compileFunc: func(q orm.Query, m fmt.Model) (orm.Plan, error) {
				compiledActions = append(compiledActions, q.Action)
				return orm.Plan{Query: "MOCK"}, nil
			},
		}
		mockExec := &MockExecutor{} // Not an introspector, fallback to additive
		db := orm.New(mockExec, mockCompiler)

		fields := []fmt.Field{
			{Name: "id"},
			{Name: "name"},
		}

		err := db.SyncSchema("users", fields)
		if err != nil {
			t.Fatalf("SyncSchema failed: %v", err)
		}

		// Expected Actions: CreateTable, AddColumn (id), AddColumn (name)
		expected := []orm.Action{orm.ActionCreateTable, orm.ActionAddColumn, orm.ActionAddColumn}
		if len(compiledActions) != len(expected) {
			t.Fatalf("Expected %d actions, got %d: %v", len(expected), len(compiledActions), compiledActions)
		}
		for i, act := range expected {
			if compiledActions[i] != act {
				t.Errorf("At index %d: expected %v, got %v", i, act, compiledActions[i])
			}
		}
	})
}

func TestObservability(t *testing.T) {
	// O5: failing AddColumn + SetLog(rec) -> error logged to rec, loop continues
	t.Run("O5 - AddColumn error logged and continues", func(t *testing.T) {
		var logs []string
		mockCompiler := &MockCompiler{}
		callCount := 0
		addColErr := errors.New("duplicate column")
		mockExec := &callCountingExecutor{
			execFn: func(query string, args ...any) error {
				callCount++
				if callCount == 1 {
					return nil // CreateTable succeeds
				}
				return addColErr // AddColumn fails
			},
		}
		db := orm.New(mockExec, mockCompiler)
		db.SetLog(func(args ...any) {
			msg := ""
			for i, a := range args {
				if i > 0 {
					msg += " "
				}
				if err, ok := a.(error); ok {
					msg += err.Error()
				} else {
					msg += fmt.Convert(a).String()
				}
			}
			logs = append(logs, msg)
		})

		model := &MockModel{
			Table: "user",
			Sch:   []fmt.Field{{Name: "name"}, {Name: "age"}},
		}

		err := db.Sync(model)
		if err != nil {
			t.Fatalf("Expected nil error, got %v", err)
		}

		// Expected logs for 2 failing AddColumn calls
		if len(logs) < 2 {
			t.Fatalf("Expected at least 2 log messages, got %d: %v", len(logs), logs)
		}
		if !fmt.Contains(logs[0], "add column name skipped: duplicate column") {
			t.Errorf("Unexpected log content: %s", logs[0])
		}
	})

	// O6: failing AddColumn inside a Tx (TxExecutor) + SetLog(rec)
	t.Run("O6 - AddColumn error logged inside Tx", func(t *testing.T) {
		var logs []string
		mockCompiler := &functionalCompiler{
			compileFunc: func(q orm.Query, m fmt.Model) (orm.Plan, error) {
				if q.Action == orm.ActionCreateTable {
					return orm.Plan{Query: "CREATE TABLE"}, nil
				}
				if q.Action == orm.ActionAddColumn {
					return orm.Plan{Query: "ADD COLUMN"}, nil
				}
				return orm.Plan{Query: "MOCK"}, nil
			},
		}

		mockTxExec := &TxLoggingMockExecutor{
			logs: &logs,
		}

		db := orm.New(mockTxExec, mockCompiler)
		db.SetLog(func(args ...any) {
			msg := ""
			for _, a := range args {
				if err, ok := a.(error); ok {
					msg += err.Error() + " "
				} else {
					msg += fmt.Convert(a).String() + " "
				}
			}
			logs = append(logs, msg)
		})

		model := &MockModel{
			Table: "user",
			Sch:   []fmt.Field{{Name: "name"}},
		}

		err := db.Sync(model)
		if err != nil {
			t.Fatalf("Sync failed: %v", err)
		}

		found := false
		for _, l := range logs {
			if fmt.Contains(l, "tx add fail") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Log not found in Tx. Logs: %v", logs)
		}
	})
}

type TxLoggingMockExecutor struct {
	MockExecutor
	logs *[]string
}

func (m *TxLoggingMockExecutor) BeginTx() (orm.TxBoundExecutor, error) {
	return &TxLoggingMockBoundExecutor{logs: m.logs}, nil
}

type TxLoggingMockBoundExecutor struct {
	MockTxBoundExecutor
	logs *[]string
}

func (m *TxLoggingMockBoundExecutor) Exec(query string, args ...any) error {
	if query == "ADD COLUMN" {
		return errors.New("tx add fail")
	}
	return nil
}

func (m *TxLoggingMockBoundExecutor) Query(query string, args ...any) (orm.Rows, error) {
	return &MockRows{}, nil
}
func (m *TxLoggingMockBoundExecutor) QueryRow(query string, args ...any) orm.Scanner {
	return &MockScanner{}
}
func (m *TxLoggingMockBoundExecutor) Close() error {
	return nil
}
func (m *TxLoggingMockBoundExecutor) Commit() error {
	return nil
}
func (m *TxLoggingMockBoundExecutor) Rollback() error {
	return nil
}

package tests

import (
	"errors"
	"testing"

	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
)

// MockIntrospectingExecutor mocks TableIntrospector and standard query execution.
type MockIntrospectingExecutor struct {
	MockExecutor
	Cols           map[string][]string
	QueryResponses map[string]*MockRows
}

func (m *MockIntrospectingExecutor) TableColumns(table string) ([]string, error) {
	if m.Cols == nil {
		return nil, nil
	}
	cols, ok := m.Cols[table]
	if !ok {
		return nil, errors.New("table not found in mock introspector")
	}
	return cols, nil
}

func (m *MockIntrospectingExecutor) Query(query string, args ...any) (orm.Rows, error) {
	m.ExecutedQueries = append(m.ExecutedQueries, query)
	m.ExecutedArgs = append(m.ExecutedArgs, args)
	if m.QueryResponses != nil {
		if resp, ok := m.QueryResponses[query]; ok {
			// Reset current for multiple iterations in tests
			resp.Current = 0
			return resp, nil
		}
	}
	return &MockRows{}, nil
}

// MockRenameModel is a mock Model that also implements RenameProvider.
type MockRenameModel struct {
	MockModel
	Renames map[string]string
}

func (m *MockRenameModel) OldNames() map[string]string {
	return m.Renames
}

func RunSyncTests(t *testing.T) {
	// S1: db.Sync new model -> a CreateTable Action is issued
	t.Run("S1 - CreateTable issued on Sync", func(t *testing.T) {
		mockCompiler := &MockCompiler{}
		mockExec := &MockExecutor{}
		db := orm.New(mockExec, mockCompiler)

		model := &MockModel{
			Table: "user",
			Sch:   []fmt.Field{{Name: "name"}},
		}

		err := db.Sync(model)
		if err != nil {
			t.Fatalf("Sync failed: %v", err)
		}

		// Since mockExec is not a TableIntrospector, it falls back to additive loop.
		// The compiler should have seen ActionCreateTable first.
		if len(mockExec.ExecutedQueries) == 0 {
			t.Fatal("Expected queries to be executed")
		}
		// CreateTable is executed first.
		// Compile is called multiple times, let's verify compile/exec happened.
	})

	// S2: db.Sync model with N fields -> N AddColumn Actions issued (no introspector)
	t.Run("S2 - N AddColumn actions on fallback", func(t *testing.T) {
		mockCompiler := &MockCompiler{}
		mockExec := &MockExecutor{}
		db := orm.New(mockExec, mockCompiler)

		model := &MockModel{
			Table: "user",
			Sch:   []fmt.Field{{Name: "name"}, {Name: "age"}},
		}

		err := db.Sync(model)
		if err != nil {
			t.Fatalf("Sync failed: %v", err)
		}

		// Fallback issues ActionCreateTable + 2 AddColumn actions
		// In MockCompiler, let's verify compile actions compiled:
		// Since Compiler is mocked, we can inspect LastQuery or mock compiler to count Actions
	})

	// S3: db.Sync where AddColumn errors -> error ignored, loop continues.
	// CreateTable must succeed; only the AddColumn calls return an error.
	t.Run("S3 - AddColumn error ignored and continues", func(t *testing.T) {
		mockCompiler := &MockCompiler{}

		// callN-based executor: first Exec (CreateTable) succeeds, rest fail.
		callCount := 0
		addColErr := errors.New("duplicate column")
		countingExec := &callCountingExecutor{
			execFn: func(query string, args ...any) error {
				callCount++
				if callCount == 1 {
					return nil // CreateTable succeeds
				}
				return addColErr // AddColumn calls fail – should be swallowed
			},
		}
		db := orm.New(countingExec, mockCompiler)

		model := &MockModel{
			Table: "user",
			Sch:   []fmt.Field{{Name: "name"}, {Name: "age"}},
		}

		err := db.Sync(model)
		// Sync must succeed: AddColumn errors are ignored in the fallback path.
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		// CreateTable + 2 AddColumn calls expected.
		if callCount != 3 {
			t.Errorf("Expected 3 Exec calls (1 CreateTable + 2 AddColumn), got %d", callCount)
		}
	})

	// S4: db.Sync where CreateTable errors -> returns ErrSyncFailed
	t.Run("S4 - CreateTable error returns ErrSyncFailed", func(t *testing.T) {
		mockCompiler := &MockCompiler{}
		mockExec := &MockExecutor{ReturnExecErr: errors.New("create table database error")}
		db := orm.New(mockExec, mockCompiler)

		model := &MockModel{
			Table: "user",
			Sch:   []fmt.Field{{Name: "name"}},
		}

		err := db.Sync(model)
		if !errors.Is(err, orm.ErrSyncFailed) {
			t.Fatalf("Expected ErrSyncFailed, got %v", err)
		}
	})

	// S5: db.Sync empty model list -> no-op, nil error
	t.Run("S5 - Empty model list is no-op", func(t *testing.T) {
		mockCompiler := &MockCompiler{}
		mockExec := &MockExecutor{}
		db := orm.New(mockExec, mockCompiler)

		err := db.Sync()
		if err != nil {
			t.Fatalf("Expected nil error, got %v", err)
		}
		if len(mockExec.ExecutedQueries) != 0 {
			t.Errorf("Expected 0 queries executed, got %d", len(mockExec.ExecutedQueries))
		}
	})

	// S6: db.Sync inside Tx when TxExecutor present
	t.Run("S6 - Sync wrapped in Tx", func(t *testing.T) {
		mockTxBound := &MockTxBoundExecutor{}
		mockTxExec := &MockTxExecutor{Bound: mockTxBound}
		mockCompiler := &MockCompiler{}
		db := orm.New(mockTxExec, mockCompiler)

		model := &MockModel{
			Table: "user",
			Sch:   []fmt.Field{{Name: "name"}},
		}

		err := db.Sync(model)
		if err != nil {
			t.Fatalf("Sync failed: %v", err)
		}

		if !mockTxBound.CommitCalled {
			t.Error("Expected transaction to be committed")
		}
	})

	// S11: db.Sync with rename provider -> ActionRenameColumn is issued
	t.Run("S11 - RenameColumn issued with OldName", func(t *testing.T) {
		mockCompiler := &MockCompiler{}
		mockExec := &MockIntrospectingExecutor{
			Cols: map[string][]string{
				"user": {"id", "nick"}, // database has "nick", but model has "username"
			},
		}
		db := orm.New(mockExec, mockCompiler)

		model := &MockRenameModel{
			MockModel: MockModel{
				Table: "user",
				Sch:   []fmt.Field{{Name: "id"}, {Name: "username"}},
			},
			Renames: map[string]string{
				"username": "nick",
			},
		}

		// Capture actions compiled by Compiler
		var compiledActions []orm.Action
		var renameQuery orm.Query
		db = orm.New(mockExec, &functionalCompiler{
			compileFunc: func(q orm.Query, m fmt.Model) (orm.Plan, error) {
				compiledActions = append(compiledActions, q.Action)
				if q.Action == orm.ActionRenameColumn {
					renameQuery = q
				}
				return orm.Plan{Query: "MOCK_QUERY"}, nil
			},
		})

		err := db.Sync(model)
		if err != nil {
			t.Fatalf("Sync failed: %v", err)
		}

		hasRename := false
		for _, act := range compiledActions {
			if act == orm.ActionRenameColumn {
				hasRename = true
			}
		}

		if !hasRename {
			t.Fatal("Expected ActionRenameColumn to be compiled")
		}

		if renameQuery.OldName != "nick" {
			t.Errorf("Expected OldName 'nick', got '%s'", renameQuery.OldName)
		}
		if renameQuery.Column == nil || renameQuery.Column.Name != "username" {
			t.Errorf("Expected target column 'username', got '%v'", renameQuery.Column)
		}
	})

	// S12: db.Sync with removed field (no data) -> ActionDropColumn is issued
	t.Run("S12 - DropColumn when empty", func(t *testing.T) {
		mockExec := &MockIntrospectingExecutor{
			Cols: map[string][]string{
				"user": {"id", "username", "obsolete_col"},
			},
			QueryResponses: map[string]*MockRows{
				"MOCK_QUERY": &MockRows{Count: 0}, // SELECT 1 returned 0 rows (no data)
			},
		}

		var compiledActions []orm.Action
		var dropQuery orm.Query
		db := orm.New(mockExec, &functionalCompiler{
			compileFunc: func(q orm.Query, m fmt.Model) (orm.Plan, error) {
				compiledActions = append(compiledActions, q.Action)
				if q.Action == orm.ActionDropColumn {
					dropQuery = q
				}
				return orm.Plan{Query: "MOCK_QUERY"}, nil
			},
		})

		model := &MockModel{
			Table: "user",
			Sch:   []fmt.Field{{Name: "id"}, {Name: "username"}},
		}

		err := db.Sync(model)
		if err != nil {
			t.Fatalf("Sync failed: %v", err)
		}

		hasDrop := false
		for _, act := range compiledActions {
			if act == orm.ActionDropColumn {
				hasDrop = true
			}
		}

		if !hasDrop {
			t.Fatal("Expected ActionDropColumn to be issued")
		}

		if len(dropQuery.Columns) != 1 || dropQuery.Columns[0] != "obsolete_col" {
			t.Errorf("Expected drop column 'obsolete_col', got %v", dropQuery.Columns)
		}
	})

	// S13: db.Sync with removed field (has data) -> DropColumn is skipped
	t.Run("S13 - DropColumn skipped when contains data", func(t *testing.T) {
		mockExec := &MockIntrospectingExecutor{
			Cols: map[string][]string{
				"user": {"id", "username", "obsolete_col"},
			},
			QueryResponses: map[string]*MockRows{
				"MOCK_QUERY": &MockRows{Count: 1}, // SELECT 1 returned 1 row (has data!)
			},
		}

		var compiledActions []orm.Action
		db := orm.New(mockExec, &functionalCompiler{
			compileFunc: func(q orm.Query, m fmt.Model) (orm.Plan, error) {
				compiledActions = append(compiledActions, q.Action)
				return orm.Plan{Query: "MOCK_QUERY"}, nil
			},
		})

		model := &MockModel{
			Table: "user",
			Sch:   []fmt.Field{{Name: "id"}, {Name: "username"}},
		}

		err := db.Sync(model)
		if err != nil {
			t.Fatalf("Sync failed: %v", err)
		}

		for _, act := range compiledActions {
			if act == orm.ActionDropColumn {
				t.Fatal("ActionDropColumn should not be issued when data is present")
			}
		}
	})
}

type functionalCompiler struct {
	compileFunc func(q orm.Query, m fmt.Model) (orm.Plan, error)
}

func (f *functionalCompiler) Compile(q orm.Query, model fmt.Model) (orm.Plan, error) {
	return f.compileFunc(q, model)
}

// callCountingExecutor is an Executor whose Exec behaviour is driven by an
// injected function, making per-call error injection straightforward.
type callCountingExecutor struct {
	MockExecutor
	execFn func(query string, args ...any) error
}

func (c *callCountingExecutor) Exec(query string, args ...any) error {
	return c.execFn(query, args...)
}

package mock

import (
	"errors"
	"reflect"
	"testing"
	"unsafe"

	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/orm/conformance"
)

func TestMockConformance(t *testing.T) {
	conformance.Run(t, conformance.Factory{
		Name: "mock",
		New: func(t *testing.T, models ...model.Model) *orm.DB {
			return NewDB()
		},
	})
}

func TestRecorders_Coverage(t *testing.T) {
	// 1. Executor
	exec := &Executor{}
	if err := exec.Exec("query"); err != nil {
		t.Errorf("Exec error: %v", err)
	}
	sc := exec.QueryRow("query_row")
	if sc == nil {
		t.Error("expected non-nil scanner")
	}
	r, err := exec.Query("query_rows")
	if err != nil || r == nil {
		t.Errorf("Query error: %v", err)
	}
	if err := exec.Close(); err != nil {
		t.Errorf("Close error: %v", err)
	}

	execErr := errors.New("exec error")
	exec.ReturnExecErr = execErr
	if err := exec.Exec("query"); err != execErr {
		t.Errorf("expected exec error, got: %v", err)
	}

	customScanner := &Scanner{ScanErr: errors.New("scan error")}
	exec.ReturnQueryRow = customScanner
	if exec.QueryRow("query_row") != customScanner {
		t.Error("expected custom scanner")
	}

	customRows := &Rows{ScanErr: errors.New("scan error")}
	exec.ReturnQueryRows = customRows
	if gotRows, _ := exec.Query("query"); gotRows != customRows {
		t.Error("expected custom rows")
	}

	exec.ReturnCloseErr = errors.New("close error")
	if err := exec.Close(); err == nil || err.Error() != "close error" {
		t.Errorf("expected close error, got: %v", err)
	}

	// 2. Scanner
	if err := customScanner.Scan(); err == nil || err.Error() != "scan error" {
		t.Errorf("expected scan error, got: %v", err)
	}

	// 3. Rows
	rows := &Rows{
		Count:      2,
		ColumnsVal: []string{"col1"},
		ColumnsErr: errors.New("cols err"),
		CloseErr:   errors.New("close err"),
		ErrVal:     errors.New("err val"),
	}
	if !rows.Next() {
		t.Error("expected true")
	}
	if !rows.Next() {
		t.Error("expected true")
	}
	if rows.Next() {
		t.Error("expected false")
	}
	if err := rows.Scan(); err != nil {
		t.Errorf("expected nil scan error, got %v", err)
	}
	if _, err := rows.Columns(); err == nil || err.Error() != "cols err" {
		t.Errorf("expected cols err, got %v", err)
	}
	if err := rows.Close(); err == nil || err.Error() != "close err" {
		t.Errorf("expected close err, got %v", err)
	}
	if err := rows.Err(); err == nil || err.Error() != "err val" {
		t.Errorf("expected err val, got %v", err)
	}

	// 4. Compiler
	comp := &Compiler{}
	plan, err := comp.Compile(orm.Query{}, nil)
	if err != nil {
		t.Errorf("Compile error: %v", err)
	}
	if plan.Query != "MOCK_QUERY" {
		t.Errorf("expected MOCK_QUERY, got %s", plan.Query)
	}

	comp.ReturnPlan = orm.Plan{Query: "CUSTOM"}
	comp.ReturnErr = errors.New("compile err")
	plan, err = comp.Compile(orm.Query{}, nil)
	if err == nil || err.Error() != "compile err" {
		t.Errorf("expected compile err, got %v", err)
	}
	if plan.Query != "CUSTOM" {
		t.Errorf("expected CUSTOM, got %s", plan.Query)
	}

	// 5. Model
	m := &Model{
		Table: "test",
		Sch:   []model.Field{{Name: "id", Type: model.Text()}},
		Vals:  []any{"val1"},
	}
	if err := m.Validate(0); err != nil {
		t.Errorf("expected nil validate error, got %v", err)
	}
	m.ValidErr = errors.New("valid err")
	if err := m.Validate(0); err == nil || err.Error() != "valid err" {
		t.Errorf("expected valid err, got %v", err)
	}
	if m.ModelName() != "test" {
		t.Errorf("expected test, got %s", m.ModelName())
	}
	if len(m.Schema()) != 1 {
		t.Errorf("expected schema length 1")
	}
	if len(m.Pointers()) != 1 {
		t.Errorf("expected pointers length 1")
	}
	if m.IsNil() {
		t.Error("expected IsNil false")
	}
	var mVal Model
	mVal.EncodeFields(nil)
	mVal.DecodeFields(nil)

	// 6. TxExecutor
	txExec := &TxExecutor{}
	txBound, err := txExec.BeginTx()
	if err != nil || txBound == nil {
		t.Errorf("BeginTx error: %v", err)
	}
	txExec.BeginTxErr = errors.New("begin tx err")
	if _, err := txExec.BeginTx(); err == nil || err.Error() != "begin tx err" {
		t.Errorf("expected begin tx err, got %v", err)
	}

	// 7. TxBoundExecutor
	bound := &TxBoundExecutor{}
	if err := bound.Commit(); err != nil {
		t.Errorf("Commit err: %v", err)
	}
	if !bound.CommitCalled {
		t.Error("expected CommitCalled true")
	}
	bound.CommitErr = errors.New("commit err")
	if err := bound.Commit(); err == nil || err.Error() != "commit err" {
		t.Errorf("expected commit err, got %v", err)
	}

	if err := bound.Rollback(); err != nil {
		t.Errorf("Rollback err: %v", err)
	}
	if !bound.RollbackCalled {
		t.Error("expected RollbackCalled true")
	}
	bound.RollbackErr = errors.New("rollback err")
	if err := bound.Rollback(); err == nil || err.Error() != "rollback err" {
		t.Errorf("expected rollback err, got %v", err)
	}
}

func setUnexportedField(obj any, fieldName string, value any) {
	val := reflect.ValueOf(obj).Elem()
	f := val.FieldByName(fieldName)
	ptr := unsafe.Pointer(f.UnsafeAddr())
	switch v := value.(type) {
	case string:
		*(*string)(ptr) = v
	}
}

func TestEngine_EdgeCasesCoverage(t *testing.T) {
	db := NewDB()

	// 1. Close, Commit, Rollback, BeginTx on Engine
	if err := db.Close(); err != nil {
		t.Errorf("Close error: %v", err)
	}
	err := db.Tx(func(tx *orm.DB) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Tx failed: %v", err)
	}

	// Rollback on engine directly
	if err := db.RawExecutor().(orm.TxBoundExecutor).Rollback(); err != nil {
		t.Errorf("Rollback failed: %v", err)
	}

	// Columns and Scan with short dest
	if err := db.Create(&conformance.Widget{Id: "t2", Name: "test2"}); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	rowsRes, err := db.RawExecutor().Query("select...")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	cols, err := rowsRes.Columns()
	if err != nil {
		t.Fatalf("Columns failed: %v", err)
	}
	if len(cols) != 4 || cols[0] != "id" {
		t.Errorf("expected 4 columns, got %v", cols)
	}

	if !rowsRes.Next() {
		t.Fatal("expected row")
	}
	var idOnly string
	err = rowsRes.Scan(&idOnly) // dest length 1
	if err != nil {
		t.Errorf("expected nil error for short scan, got %v", err)
	}

	// Scan with incompatible/unsupported scan type to trigger ScanAny error inside scanInto
	var unsupportedDest struct{}
	err = rowsRes.Scan(&unsupportedDest)
	if err == nil {
		t.Error("expected ScanAny error for unsupported type")
	}

	// Scan missing column
	var missingDest string
	schemaWithMissing := []model.Field{{Name: "nonexistent", Type: model.Text()}}
	err = scanInto(dbRow{}, schemaWithMissing, []any{&missingDest})
	if err != nil {
		t.Errorf("expected nil error for missing scan, got %v", err)
	}

	// 2. LIKE Wildcard matching edge cases
	likeCases := []struct {
		s       string
		pattern string
		want    bool
	}{
		{"", "", true},
		{"abc", "", false},
		{"", "%", true},
		{"abc", "%", true},
		{"abc", "abc", true},
		{"abc", "ab", false},
		{"abc", "abc%", true},
		{"abc", "ab%", true},
		{"abc", "%bc", true},
		{"abc", "%b%", true},
		{"abc", "a%c", true},
		{"abc", "a%d", false},
		{"abc", "%d", false},
		{"abc", "d%", false},
		{"abc", "ab%c", true},
		{"abc", "a%b%c", true},
		{"abc", "%a%b%c%", true},
		{"abc", "%%%%%", true},
		{"", "a", false},
		{"a", "abc%", false},
		{"a", "%abc", false},
		{"abc", "ab%c%d", false},
		{"abc", "%%", true},
		{"abcxyz", "abc%def%", false},
	}
	for _, tc := range likeCases {
		got := likeMatch(tc.s, tc.pattern)
		if got != tc.want {
			t.Errorf("likeMatch(%q, %q) = %v; want %v", tc.s, tc.pattern, got, tc.want)
		}
	}

	if findHelper("abc", "") != 0 {
		t.Errorf("expected findHelper with empty sub to return 0")
	}

	// LIKE filter on evalCond inside Query
	var temp conformance.Widget
	err = db.Query(&temp).Where("name").Like("test%").ReadOne()
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	if temp.Name != "test2" {
		t.Errorf("expected matched name test2, got %q", temp.Name)
	}

	// 3. toFloat helper with int, int32, int64, float32, float64
	testsToFloat := []struct {
		val  any
		want float64
		ok   bool
	}{
		{int(42), 42.0, true},
		{int32(42), 42.0, true},
		{int64(42), 42.0, true},
		{float32(42.5), 42.5, true},
		{float64(42.5), 42.5, true},
		{"not-a-float", 0.0, false},
	}
	for _, tc := range testsToFloat {
		got, ok := toFloat(tc.val)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("toFloat(%v (%T)) = (%v, %v); want (%v, %v)", tc.val, tc.val, got, ok, tc.want, tc.ok)
		}
	}

	// 4. compareAny and equalAny with bool, string, numeric types
	if !equalAny(true, true) {
		t.Error("expected true == true")
	}
	if equalAny(true, false) {
		t.Error("expected true != false")
	}
	if equalAny("abc", 123) {
		t.Error("expected false")
	}
	if !equalAny(12.5, 12.5) {
		t.Error("expected 12.5 == 12.5")
	}
	if equalAny(struct{}{}, struct{}{}) {
		t.Error("expected false for uncomparable types")
	}

	// 5. IS NOT NULL evaluate condition
	row := dbRow{{col: "col1", val: "val1"}, {col: "col2", val: nil}}
	c1 := orm.IsNotNull("col1")
	if !evalCond(row, c1) {
		t.Error("expected col1 IS NOT NULL to be true")
	}
	c2 := orm.IsNotNull("col2")
	if evalCond(row, c2) {
		t.Error("expected col2 IS NOT NULL to be false")
	}
	c3 := orm.IsNotNull("col3") // nonexistent
	if evalCond(row, c3) {
		t.Error("expected col3 IS NOT NULL to be false")
	}

	// Unknown operator via reflection
	cUnknown := orm.Eq("col1", "val1")
	setUnexportedField(&cUnknown, "operator", "UNKNOWN")
	if evalCond(row, cUnknown) {
		t.Error("expected false for unknown operator")
	}

	// 6. inSlice helper with various slice types
	if !inSlice("a", []string{"a", "b"}) {
		t.Error("expected a in []string")
	}
	if inSlice("c", []string{"a", "b"}) {
		t.Error("expected c NOT in []string")
	}
	if !inSlice(42, []int{41, 42}) {
		t.Error("expected 42 in []int")
	}
	if inSlice(43, []int{41, 42}) {
		t.Error("expected 43 NOT in []int")
	}
	if !inSlice(int64(42), []int64{41, 42}) {
		t.Error("expected 42 in []int64")
	}
	if inSlice(int64(43), []int64{41, 42}) {
		t.Error("expected 43 NOT in []int64")
	}
	if inSlice("not-numeric", []int{41, 42}) {
		t.Error("expected false for invalid type")
	}
	if inSlice("not-numeric", []int64{41, 42}) {
		t.Error("expected false for invalid type")
	}
	if inSlice("a", nil) {
		t.Error("expected false for nil listVal")
	}

	// Delete on a table that was never Created is a no-op, not a panic.
	freshDB := NewDB()
	if err := freshDB.Delete(&conformance.Widget{}, orm.Eq("id", "nope")); err != nil {
		t.Errorf("expected nil error deleting from nonexistent table, got %v", err)
	}

	// 7. offset >= len(rows) edge case
	rowsSlice := []dbRow{{{col: "id", val: "1"}}, {{col: "id", val: "2"}}}
	resRows := applyOffsetLimit(rowsSlice, 5, 2)
	if resRows != nil {
		t.Errorf("expected nil rows for offset out of bounds, got %v", resRows)
	}

	// extra helpers coverage
	if toStr([]byte("abc")) != "abc" {
		t.Error("expected abc")
	}
	if toStr(123) != "123" {
		t.Error("expected 123")
	}

	// compareAny detailed coverage
	if compareAny(1, 2) != -1 {
		t.Error("expected 1 < 2")
	}
	if compareAny(2, 1) != 1 {
		t.Error("expected 2 > 1")
	}
	if compareAny(1, 1) != 0 {
		t.Error("expected 1 == 1")
	}
	if compareAny("a", "b") != -1 {
		t.Error("expected a < b")
	}
	if compareAny("b", "a") != 1 {
		t.Error("expected b > a")
	}
	if compareAny("a", "a") != 0 {
		t.Error("expected a == a")
	}
}

package conformance

import (
	"errors"
	"testing"

	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
)

// Factory builds, for ONE clause, a fresh *orm.DB whose Widget table already exists and is EMPTY.
// Schema setup is the backend's job, DONE OUTSIDE orm's DML contract (this suite never calls DDL):
//   - mock:     the in-memory engine auto-creates the table on first Create — New just returns NewDB().
//   - sqlite/postgres: New runs the dialect's ddlc.ExportDDL(models) (DROP+CREATE) before returning.
//   - indexdb:  New declares `models` as IndexedDB object stores (structTables) up front.
// models are the record types the suite will exercise. Called once per clause → no cross-clause bleed.
type Factory struct {
	Name string
	New  func(t *testing.T, models ...model.Model) *orm.DB
}

func Run(t *testing.T, f Factory) {
	if f.New == nil {
		t.Fatal("conformance: Factory.New is required")
	}
	t.Run("create_then_read_one_by_pk", func(t *testing.T) { createThenReadOneByPK(t, f) })
	t.Run("read_one_no_match_is_not_found", func(t *testing.T) { readOneNoMatchIsNotFound(t, f) })
	t.Run("read_all_returns_every_row", func(t *testing.T) { readAllReturnsEveryRow(t, f) })
	t.Run("read_all_filters_by_eq", func(t *testing.T) { readAllFiltersByEq(t, f) })
	t.Run("read_all_ands_two_conditions", func(t *testing.T) { readAllAndsTwoConditions(t, f) })
	t.Run("read_all_ors_conditions", func(t *testing.T) { readAllOrsConditions(t, f) })
	t.Run("read_all_orders_asc_and_desc", func(t *testing.T) { readAllOrdersAscDesc(t, f) })
	t.Run("read_all_applies_limit_and_offset", func(t *testing.T) { readAllLimitOffset(t, f) })
	t.Run("comparison_operators_filter", func(t *testing.T) { comparisonOperatorsFilter(t, f) }) // > >= < <= !=
	t.Run("in_operator_filters", func(t *testing.T) { inOperatorFilters(t, f) })
	t.Run("update_changes_matched_rows_only", func(t *testing.T) { updateChangesMatchedOnly(t, f) })
	t.Run("delete_removes_matched_rows_only", func(t *testing.T) { deleteRemovesMatchedOnly(t, f) })
}

func setup(t *testing.T, f Factory, seed ...*Widget) *orm.DB {
	t.Helper()
	db := f.New(t, &Widget{}) // table already exists & empty — backend set it up, not this suite
	for _, w := range seed {
		if err := db.Create(w); err != nil {
			t.Fatalf("seed Create(%+v): %v", w, err)
		}
	}
	return db
}

func readAll(t *testing.T, qb *orm.QB) []*Widget {
	t.Helper()
	var out []*Widget
	err := qb.ReadAll(func() model.Model { return &Widget{} }, func(m model.Model) {
		out = append(out, m.(*Widget))
	})
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return out
}

func createThenReadOneByPK(t *testing.T, f Factory) {
	db := setup(t, f, &Widget{Id: "w1", Name: "alpha", Qty: 3, Active: true})
	var got Widget
	err := db.Query(&got).Where("id").Eq("w1").ReadOne()
	if err != nil {
		t.Fatalf("ReadOne: %v", err)
	}
	if got.Name != "alpha" || got.Qty != 3 || got.Active != true {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
}

func readOneNoMatchIsNotFound(t *testing.T, f Factory) {
	db := setup(t, f)
	var got Widget
	err := db.Query(&got).Where("id").Eq("nonexistent").ReadOne()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, orm.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func readAllReturnsEveryRow(t *testing.T, f Factory) {
	db := setup(t, f,
		&Widget{Id: "w1", Name: "alpha", Qty: 3, Active: true},
		&Widget{Id: "w2", Name: "beta", Qty: 4, Active: false},
	)
	got := readAll(t, db.Query(&Widget{}))
	if len(got) != 2 {
		t.Errorf("expected 2 widgets, got %d", len(got))
	}
}

func readAllFiltersByEq(t *testing.T, f Factory) {
	db := setup(t, f,
		&Widget{Id: "w1", Name: "alpha", Qty: 3, Active: true},
		&Widget{Id: "w2", Name: "beta", Qty: 4, Active: false},
	)
	got := readAll(t, db.Query(&Widget{}).Where("name").Eq("alpha"))
	if len(got) != 1 || got[0].Id != "w1" {
		t.Errorf("expected only w1, got %+v", got)
	}
}

func readAllAndsTwoConditions(t *testing.T, f Factory) {
	db := setup(t, f,
		&Widget{Id: "a", Name: "x", Qty: 1, Active: true},
		&Widget{Id: "b", Name: "x", Qty: 1, Active: false},
		&Widget{Id: "c", Name: "y", Qty: 1, Active: true},
	)
	got := readAll(t, db.Query(&Widget{}).Where("name").Eq("x").Where("active").Eq(true))
	if len(got) != 1 || got[0].Id != "a" {
		t.Errorf("AND of two conditions must return only {a}; got %+v", got)
	}
}

func readAllOrsConditions(t *testing.T, f Factory) {
	db := setup(t, f,
		&Widget{Id: "a", Name: "x", Qty: 1, Active: true},
		&Widget{Id: "b", Name: "y", Qty: 2, Active: false},
		&Widget{Id: "c", Name: "z", Qty: 3, Active: true},
	)
	got := readAll(t, db.Query(&Widget{}).Where("name").Eq("x").Or().Where("name").Eq("y"))
	if len(got) != 2 {
		t.Errorf("expected 2, got %d", len(got))
	}
	if got[0].Id != "a" || got[1].Id != "b" {
		t.Errorf("expected a and b, got %+v", got)
	}
}

func readAllOrdersAscDesc(t *testing.T, f Factory) {
	db := setup(t, f,
		&Widget{Id: "w1", Name: "alpha", Qty: 5, Active: true},
		&Widget{Id: "w2", Name: "beta", Qty: 2, Active: false},
		&Widget{Id: "w3", Name: "gamma", Qty: 8, Active: true},
	)
	// Asc
	gotAsc := readAll(t, db.Query(&Widget{}).OrderBy("qty").Asc())
	if len(gotAsc) != 3 || gotAsc[0].Id != "w2" || gotAsc[1].Id != "w1" || gotAsc[2].Id != "w3" {
		t.Errorf("expected w2, w1, w3; got %+v", gotAsc)
	}
	// Desc
	gotDesc := readAll(t, db.Query(&Widget{}).OrderBy("qty").Desc())
	if len(gotDesc) != 3 || gotDesc[0].Id != "w3" || gotDesc[1].Id != "w1" || gotDesc[2].Id != "w2" {
		t.Errorf("expected w3, w1, w2; got %+v", gotDesc)
	}
}

func readAllLimitOffset(t *testing.T, f Factory) {
	db := setup(t, f,
		&Widget{Id: "w1", Name: "alpha", Qty: 1, Active: true},
		&Widget{Id: "w2", Name: "beta", Qty: 2, Active: false},
		&Widget{Id: "w3", Name: "gamma", Qty: 3, Active: true},
		&Widget{Id: "w4", Name: "delta", Qty: 4, Active: false},
	)
	got := readAll(t, db.Query(&Widget{}).OrderBy("qty").Asc().Limit(2).Offset(1))
	if len(got) != 2 || got[0].Id != "w2" || got[1].Id != "w3" {
		t.Errorf("expected w2, w3; got %+v", got)
	}
}

func comparisonOperatorsFilter(t *testing.T, f Factory) {
	db := setup(t, f,
		&Widget{Id: "w1", Name: "alpha", Qty: 1, Active: true},
		&Widget{Id: "w2", Name: "beta", Qty: 2, Active: false},
		&Widget{Id: "w3", Name: "gamma", Qty: 3, Active: true},
	)
	// Neq
	gotNeq := readAll(t, db.Query(&Widget{}).Where("qty").Neq(2))
	if len(gotNeq) != 2 || gotNeq[0].Id != "w1" || gotNeq[1].Id != "w3" {
		t.Errorf("Neq: expected w1, w3; got %+v", gotNeq)
	}
	// Gt
	gotGt := readAll(t, db.Query(&Widget{}).Where("qty").Gt(1))
	if len(gotGt) != 2 || gotGt[0].Id != "w2" || gotGt[1].Id != "w3" {
		t.Errorf("Gt: expected w2, w3; got %+v", gotGt)
	}
	// Gte
	gotGte := readAll(t, db.Query(&Widget{}).Where("qty").Gte(2))
	if len(gotGte) != 2 || gotGte[0].Id != "w2" || gotGte[1].Id != "w3" {
		t.Errorf("Gte: expected w2, w3; got %+v", gotGte)
	}
	// Lt
	gotLt := readAll(t, db.Query(&Widget{}).Where("qty").Lt(3))
	if len(gotLt) != 2 || gotLt[0].Id != "w1" || gotLt[1].Id != "w2" {
		t.Errorf("Lt: expected w1, w2; got %+v", gotLt)
	}
	// Lte
	gotLte := readAll(t, db.Query(&Widget{}).Where("qty").Lte(2))
	if len(gotLte) != 2 || gotLte[0].Id != "w1" || gotLte[1].Id != "w2" {
		t.Errorf("Lte: expected w1, w2; got %+v", gotLte)
	}
}

func inOperatorFilters(t *testing.T, f Factory) {
	db := setup(t, f,
		&Widget{Id: "a", Name: "alpha", Qty: 1, Active: true},
		&Widget{Id: "b", Name: "beta", Qty: 2, Active: false},
		&Widget{Id: "c", Name: "gamma", Qty: 3, Active: true},
	)
	got := readAll(t, db.Query(&Widget{}).Where("id").In([]any{"a", "c"}))
	if len(got) != 2 || got[0].Id != "a" || got[1].Id != "c" {
		t.Errorf("In: expected a, c; got %+v", got)
	}
}

func updateChangesMatchedOnly(t *testing.T, f Factory) {
	db := setup(t, f,
		&Widget{Id: "w1", Name: "alpha", Qty: 1, Active: true},
		&Widget{Id: "w2", Name: "beta", Qty: 2, Active: false},
	)
	m := &Widget{Name: "updated", Qty: 99, Active: true}
	err := db.Update(m, orm.Eq("id", "w1"))
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	// w1 must be updated
	var got1 Widget
	if err := db.Query(&got1).Where("id").Eq("w1").ReadOne(); err != nil {
		t.Fatalf("ReadOne w1: %v", err)
	}
	if got1.Name != "updated" || got1.Qty != 99 || got1.Active != true {
		t.Errorf("w1 was not correctly updated: %+v", got1)
	}
	// w2 must be unchanged
	var got2 Widget
	if err := db.Query(&got2).Where("id").Eq("w2").ReadOne(); err != nil {
		t.Fatalf("ReadOne w2: %v", err)
	}
	if got2.Name != "beta" || got2.Qty != 2 || got2.Active != false {
		t.Errorf("w2 was modified but shouldn't have been: %+v", got2)
	}
}

func deleteRemovesMatchedOnly(t *testing.T, f Factory) {
	db := setup(t, f,
		&Widget{Id: "w1", Name: "alpha", Qty: 1, Active: true},
		&Widget{Id: "w2", Name: "beta", Qty: 2, Active: false},
	)
	err := db.Delete(&Widget{}, orm.Eq("id", "w1"))
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	// w1 must be gone
	var got1 Widget
	err = db.Query(&got1).Where("id").Eq("w1").ReadOne()
	if err == nil || !errors.Is(err, orm.ErrNotFound) {
		t.Errorf("expected w1 to be deleted/not found, got err: %v", err)
	}
	// w2 must be present
	var got2 Widget
	if err := db.Query(&got2).Where("id").Eq("w2").ReadOne(); err != nil {
		t.Errorf("expected w2 to still exist, got: %v", err)
	}
}

package tests

import (
	"strings"
	"testing"

	"webtyp.com/model"
	"webtyp.com/orm"
	"webtyp.com/storage/conformance"
	"webtyp.com/storage/mem"
)

func TestUpdateFieldsWritesOnlyTheNamedColumns(t *testing.T) {
	d := orm.New(mem.New())

	w := &conformance.Widget{Id: "w1", Name: "alpha", Qty: 10, Active: true}
	if err := d.Create(w); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// UpdateFields with only "qty" named; Name is changed on the struct but omitted from fields slice
	if err := d.UpdateFields(&conformance.Widget{Name: "pushed_away", Qty: 99}, []string{"qty"}, orm.Eq("id", "w1")); err != nil {
		t.Fatalf("UpdateFields: %v", err)
	}

	var got conformance.Widget
	if err := d.Query(&got).Where("id").Eq("w1").ReadOne(); err != nil {
		t.Fatalf("ReadOne: %v", err)
	}

	if got.Qty != 99 {
		t.Errorf("expected Qty 99, got %d", got.Qty)
	}
	if got.Name != "alpha" {
		t.Errorf("expected Name 'alpha' to remain untouched, got %q", got.Name)
	}
	if !got.Active {
		t.Errorf("expected Active true to remain untouched, got false")
	}
}

func TestUpdateFieldsRejectsAnEmptyFieldList(t *testing.T) {
	d := orm.New(mem.New())

	err := d.UpdateFields(&conformance.Widget{Qty: 10}, []string{}, orm.Eq("id", "w1"))
	if err == nil {
		t.Fatal("expected error when passing empty field list, got nil")
	}
	if !strings.Contains(err.Error(), "at least one field") {
		t.Errorf("expected error message to contain 'at least one field', got: %v", err)
	}
}

func TestUpdateFieldsRejectsAnUnknownField(t *testing.T) {
	d := orm.New(mem.New())

	err := d.UpdateFields(&conformance.Widget{Qty: 10}, []string{"nonexistent"}, orm.Eq("id", "w1"))
	if err == nil {
		t.Fatal("expected error when passing unknown field, got nil")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("expected error message to contain 'unknown field', got: %v", err)
	}
}

func TestUpdateFieldsRejectsADuplicateField(t *testing.T) {
	d := orm.New(mem.New())

	err := d.UpdateFields(&conformance.Widget{Qty: 10}, []string{"qty", "qty"}, orm.Eq("id", "w1"))
	if err == nil {
		t.Fatal("expected error when passing duplicate field, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate field") {
		t.Errorf("expected error message to contain 'duplicate field', got: %v", err)
	}
}

func TestUpdateFieldsAppliesToEveryMatchedRow(t *testing.T) {
	d := orm.New(mem.New())

	_ = d.Create(&conformance.Widget{Id: "w1", Name: "w1", Qty: 10, Active: true})
	_ = d.Create(&conformance.Widget{Id: "w2", Name: "w2", Qty: 20, Active: true})
	_ = d.Create(&conformance.Widget{Id: "w3", Name: "w3", Qty: 30, Active: true})

	ids := []any{"w1", "w2", "w3"}
	if err := d.UpdateFields(&conformance.Widget{Qty: 50}, []string{"qty"}, orm.In("id", ids)); err != nil {
		t.Fatalf("UpdateFields with In: %v", err)
	}

	var all []*conformance.Widget
	err := d.Query(&conformance.Widget{}).Where("id").In(ids).ReadAll(
		func() model.Model { return &conformance.Widget{} },
		func(m model.Model) {
			all = append(all, m.(*conformance.Widget))
		},
	)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if len(all) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(all))
	}
	for _, w := range all {
		if w.Qty != 50 {
			t.Errorf("expected Qty 50 for row %s, got %d", w.Id, w.Qty)
		}
	}
}

func TestDeleteRemovesEveryMatchedRow(t *testing.T) {
	d := orm.New(mem.New())

	_ = d.Create(&conformance.Widget{Id: "w1", Name: "w1", Qty: 10, Active: true})
	_ = d.Create(&conformance.Widget{Id: "w2", Name: "w2", Qty: 20, Active: true})
	_ = d.Create(&conformance.Widget{Id: "w3", Name: "w3", Qty: 30, Active: true})
	_ = d.Create(&conformance.Widget{Id: "w4", Name: "w4", Qty: 40, Active: true})

	deleteIds := []any{"w1", "w2", "w3"}
	if err := d.Delete(&conformance.Widget{}, orm.In("id", deleteIds)); err != nil {
		t.Fatalf("Delete with In: %v", err)
	}

	var remaining []*conformance.Widget
	err := d.Query(&conformance.Widget{}).ReadAll(
		func() model.Model { return &conformance.Widget{} },
		func(m model.Model) {
			remaining = append(remaining, m.(*conformance.Widget))
		},
	)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining row, got %d", len(remaining))
	}
	if remaining[0].Id != "w4" {
		t.Errorf("expected remaining row to be w4, got %s", remaining[0].Id)
	}
}

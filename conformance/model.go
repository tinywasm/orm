package conformance

import (
	"github.com/tinywasm/model"
)

// Widget is the canonical record every backend is driven with. Its schema carries real DB
// metadata (types + PK) so SQL backends can CREATE TABLE it; the mock ignores the metadata
// and stores by column name. Hand-written (conformance depends only on model, not ormc).
var WidgetModel = model.Definition{
	Name: "conformance_widget",
	Fields: model.Fields{
		{Name: "id", Type: model.Text(), DB: &model.FieldDB{PK: true}},
		{Name: "name", Type: model.Text(), NotNull: true},
		{Name: "qty", Type: model.Int(), NotNull: true},
		{Name: "active", Type: model.Bool(), NotNull: true},
	},
}

type Widget struct {
	Id     string
	Name   string
	Qty    int64
	Active bool
}

func (w *Widget) ModelName() string     { return WidgetModel.Name }
func (w *Widget) Schema() []model.Field  { return WidgetModel.Fields }
func (w *Widget) Pointers() []any        { return []any{&w.Id, &w.Name, &w.Qty, &w.Active} }
func (w *Widget) IsNil() bool            { return w == nil }
func (w *Widget) EncodeFields(wr model.FieldWriter) {
	wr.String("id", w.Id); wr.String("name", w.Name); wr.Int("qty", w.Qty); wr.Bool("active", w.Active)
}
func (w *Widget) DecodeFields(r model.FieldReader) {
	if v, ok := r.String("id"); ok { w.Id = v }
	if v, ok := r.String("name"); ok { w.Name = v }
	if v, ok := r.Int("qty"); ok { w.Qty = v }
	if v, ok := r.Bool("active"); ok { w.Active = v }
}

var _ model.Model = (*Widget)(nil)

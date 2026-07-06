package ormc

import "github.com/tinywasm/model"


// SchemaSyncer applies a parsed table schema. Implemented by the consumer
// (tinywasm/app) over *orm.DB; ormc only ever sees this interface.
type SchemaSyncer interface {
	SyncSchema(table string, fields []model.Field) error
}

// SetSyncer sets the schema syncer for the generator.
func (g *Generator) SetSyncer(s SchemaSyncer) {
	g.syncer = s
}

package ddl

import "github.com/tinywasm/model"

// Exporter is implemented by SQL adapter compilers (sqlt, postgres).
// ExportDDL returns CREATE TABLE + index statements for all models, in FK dependency order.
type Exporter interface {
	ExportDDL(models []model.Model) (string, error)
}

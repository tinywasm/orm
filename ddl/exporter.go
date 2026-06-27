package ddl

import "github.com/tinywasm/fmt"

// Exporter is implemented by SQL adapter compilers (sqlt, postgres).
// ExportDDL returns CREATE TABLE + index statements for all models, in FK dependency order.
type Exporter interface {
	ExportDDL(models []fmt.Model) (string, error)
}


package ormc

import "github.com/tinywasm/modfind"

// Ormc is the code generator handler for the ormc tool.
type Generator struct {
	logFn    func(messages ...any)
	rootDir  string
	skipTidy bool
	syncer   SchemaSyncer
	finder   *modfind.Finder
	cache    map[string]StructInfo
}

// NewOrmc creates a new Ormc handler with rootDir defaulting to ".".
func New() *Generator {
	return &Generator{
		rootDir: ".",
		cache:   make(map[string]StructInfo),
	}
}

// SetSkipTidy enables or disables the go mod tidy pass.
func (g *Generator) SetSkipTidy(skip bool) {
	g.skipTidy = skip
}

// SetLog sets the log function for warnings and informational messages.
// If not set, messages are silently discarded.
func (g *Generator) SetLog(fn func(messages ...any)) {
	g.logFn = fn
}

// SetRootDir sets the root directory that Run() will scan.
// Defaults to ".". Useful in tests to point to a specific directory
// without needing os.Chdir.
func (g *Generator) SetRootDir(dir string) {
	g.rootDir = dir
}

// SetFinder injects the shared modfind.Finder (one go list across ssr/image/ormc).
func (g *Generator) SetFinder(f *modfind.Finder) {
	g.finder = f
}

// Name returns the TUI handler label shown in the development console.
func (g *Generator) Name() string { return "ORMC" }

// log emits a message via the configured log function, if any.
func (g *Generator) log(messages ...any) {
	if g.logFn != nil {
		g.logFn(messages...)
	}
}

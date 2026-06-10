
package ormc

// Ormc is the code generator handler for the ormc tool.
type Generator struct {
	logFn    func(messages ...any)
	rootDir  string
	skipTidy bool
}

// NewOrmc creates a new Ormc handler with rootDir defaulting to ".".
func New() *Generator {
	return &Generator{rootDir: "."}
}

// SetSkipTidy enables or disables the go mod tidy pass.
func (g *Generator) SetSkipTidy(skip bool) {
	g.skipTidy = skip
}

// SetLog sets the log function for warnings and informational messages.
// If not set, messages are silently discarded.
func (o *Generator) SetLog(fn func(messages ...any)) {
	o.logFn = fn
}

// SetRootDir sets the root directory that Run() will scan.
// Defaults to ".". Useful in tests to point to a specific directory
// without needing os.Chdir.
func (o *Generator) SetRootDir(dir string) {
	o.rootDir = dir
}

// log emits a message via the configured log function, if any.
func (o *Generator) log(messages ...any) {
	if o.logFn != nil {
		o.logFn(messages...)
	}
}

//go:build !wasm

package orm

// Ormc is the code generator handler for the ormc tool.
type Ormc struct {
	logFn   func(messages ...any)
	rootDir string
}

// NewOrmc creates a new Ormc handler with rootDir defaulting to ".".
func NewOrmc() *Ormc {
	return &Ormc{rootDir: "."}
}

// SetLog sets the log function for warnings and informational messages.
// If not set, messages are silently discarded.
func (o *Ormc) SetLog(fn func(messages ...any)) {
	o.logFn = fn
}

// SetRootDir sets the root directory that Run() will scan.
// Defaults to ".". Useful in tests to point to a specific directory
// without needing os.Chdir.
func (o *Ormc) SetRootDir(dir string) {
	o.rootDir = dir
}

// log emits a message via the configured log function, if any.
func (o *Ormc) log(messages ...any) {
	if o.logFn != nil {
		o.logFn(messages...)
	}
}

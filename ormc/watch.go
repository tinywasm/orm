package ormc

// NewFileEvent implements the file-event contract for watchers (e.g. tinywasm/app's devwatch).
func (g *Generator) NewFileEvent(fileName, extension, filePath, event string) error {
	if fileName == "model.go" || fileName == "models.go" {
		return g.Run()
	}
	return nil
}

// SupportedExtensions returns the list of file extensions this generator handles.
func (g *Generator) SupportedExtensions() []string {
	return []string{".go"}
}

// UnobservedFiles returns a list of files that should be ignored by the watcher.
func (g *Generator) UnobservedFiles() []string {
	return nil
}

// MainInputFileRelativePath returns the relative path to the main input file, if any.
func (g *Generator) MainInputFileRelativePath() string {
	return ""
}

package ormc

// NewFileEvent implements the file-event contract for watchers (e.g. tinywasm/app's devwatch).
func (g *Generator) NewFileEvent(fileName, extension, filePath, event string) error {
	if fileName != "model.go" && fileName != "models.go" {
		return nil
	}

	// 1. Parse only that file
	infos, err := g.parseDefinitionsInFile(filePath)
	if err != nil {
		return err
	}

	// 2. Merge into cache
	for _, info := range infos {
		g.cache[info.Name] = info
	}

	// 3. Regenerate just <file>_orm.go
	if err := g.GenerateForFile(infos, filePath); err != nil {
		return err
	}

	// 4. Sync each DB struct
	if g.syncer != nil {
		for _, info := range infos {
			if info.NoDB {
				continue
			}
			if err := g.syncer.SyncSchema(info.ModelName, info.asFields()); err != nil {
				g.log("sync error for", info.ModelName, ":", err)
			}
		}
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

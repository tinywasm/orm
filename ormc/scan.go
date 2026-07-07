package ormc

import (
	"os"
	"path/filepath"

	"github.com/tinywasm/modfind"
)

// ScanModules syncs the DB schema of every discovered module to the injected
// SchemaSyncer. Called once at startup by the tool (app), after SetSyncer.
//   - Writable module (main / replace): regenerate <file>_orm.go from model.go,
//     then sync each DB struct.
//   - Read-only module (cache): parse the committed model_orm.go, then sync.
// No-op if no syncer is injected (CLI codegen-only path).
func (g *Generator) ScanModules(rootDir string) error {
	if g.syncer == nil {
		g.log("ready (no db syncer)")
		return nil
	}
	g.log("scanning modules...")
	if g.finder == nil {
		g.finder = modfind.New()
	}
	mods, err := g.finder.Discover(rootDir)
	if err != nil {
		return err
	}
	for _, m := range mods {
		if m.Writable() {
			if err := g.scanWritableModule(m.Dir); err != nil {
				g.log("ormc scan (writable)", m.Path, ":", err) // log-and-continue
			}
		} else {
			if err := g.scanReadonlyModule(m.Dir); err != nil {
				g.log("ormc scan (readonly)", m.Path, ":", err)
			}
		}
	}
	return nil
}

func (g *Generator) scanWritableModule(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "vendor" || info.Name() == ".git" || info.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}

		fileName := info.Name()
		if fileName == "model.go" || fileName == "models.go" {
			infos, err := g.parseDefinitionsInFile(path)
			if err != nil {
				return nil // skip unparseable
			}

			// Merge into cache
			for _, info := range infos {
				g.cache[info.Name] = info
			}

			// Generate in-place
			if err := g.GenerateForFile(infos, path); err != nil {
				return err
			}

			// Sync
			for _, info := range infos {
				if !info.NoDB {
					if err := g.syncer.SyncSchema(info.ModelName, info.asFields()); err != nil {
						g.log("sync error for", info.ModelName, ":", err)
					}
				}
			}
		}
		return nil
	})
}

func (g *Generator) scanReadonlyModule(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "vendor" || info.Name() == ".git" || info.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}

		if filepath.Ext(path) == ".go" && (len(path) > 7 && path[len(path)-7:] == "_orm.go") {
			schemas, err := parseGenerated(path)
			if err != nil {
				g.log("failed to parse generated file", path, ":", err)
				return nil
			}
			for table, fields := range schemas {
				if err := g.syncer.SyncSchema(table, fields); err != nil {
					g.log("sync error (readonly) for", table, ":", err)
				}
			}
		}
		return nil
	})
}

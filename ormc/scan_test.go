package ormc

import "github.com/tinywasm/model"

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinywasm/modfind"
)

type mockSyncer struct {
	synced map[string][]model.Field
}

func (s *mockSyncer) SyncSchema(table string, fields []model.Field) error {
	if s.synced == nil {
		s.synced = make(map[string][]model.Field)
	}
	s.synced[table] = fields
	return nil
}

func TestScanModules(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "ormc_scan_test")
	defer os.RemoveAll(tmpDir)

	// Writable module
	writableDir := filepath.Join(tmpDir, "writable")
	os.MkdirAll(writableDir, 0755)
	os.WriteFile(filepath.Join(writableDir, "model.go"), []byte(`package main
import "github.com/tinywasm/model"
var UserModel = model.Definition{
	Name: "user",
	Fields: model.Fields{
		{Name: "id", Type: model.FieldText, DB: &model.FieldDB{PK: true}},
		{Name: "name", Type: model.FieldText},
	},
}
`), 0644)

	// Read-only module
	readonlyDir := filepath.Join(tmpDir, "readonly")
	os.MkdirAll(readonlyDir, 0755)
	os.WriteFile(filepath.Join(readonlyDir, "model_orm.go"), []byte(`package readonly
import "github.com/tinywasm/fmt"
func (m *Item) ModelName() string { return "items" }
var _schemaItem = []model.Field{{Name: "id", Type: model.FieldText}}
`), 0644)

	// Set an old mtime for readonly file to verify it's not rewritten
	oldTime := time.Now().Add(-1 * time.Hour)
	os.Chtimes(filepath.Join(readonlyDir, "model_orm.go"), oldTime, oldTime)

	g := New()
	syncer := &mockSyncer{}
	g.SetSyncer(syncer)

	finder := modfind.New()
	finder.Seed(tmpDir, []modfind.Module{
		{Path: "main", Dir: writableDir, IsMain: true},
		{Path: "readonly", Dir: readonlyDir, IsMain: false},
	})
	g.SetFinder(finder)

	err := g.ScanModules(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// MS3: Assert both synced
	if _, ok := syncer.synced["user"]; !ok {
		t.Error("user table (writable) not synced")
	}
	if _, ok := syncer.synced["items"]; !ok {
		t.Error("items table (readonly) not synced")
	}

	// Writable should have generated _orm.go
	if _, err := os.Stat(filepath.Join(writableDir, "model_orm.go")); os.IsNotExist(err) {
		t.Error("model_orm.go not generated in writable module")
	}

	// MS4: Read-only should NOT have been rewritten
	info, _ := os.Stat(filepath.Join(readonlyDir, "model_orm.go"))
	if info.ModTime().After(oldTime) {
		t.Error("readonly model_orm.go was rewritten")
	}
}

func TestScanModules_NoSyncer(t *testing.T) {
	// MS6: no syncer injected -> no-op
	g := New()
	err := g.ScanModules(".")
	if err != nil {
		t.Fatal(err)
	}
}

func TestScanModules_EmptyReadonly(t *testing.T) {
	// MS5: read-only module with no *_orm.go -> skipped
	tmpDir, _ := os.MkdirTemp("", "ormc_scan_empty")
	defer os.RemoveAll(tmpDir)

	readonlyDir := filepath.Join(tmpDir, "readonly")
	os.MkdirAll(readonlyDir, 0755)

	g := New()
	syncer := &mockSyncer{}
	g.SetSyncer(syncer)

	finder := modfind.New()
	finder.Seed(tmpDir, []modfind.Module{
		{Path: "readonly", Dir: readonlyDir, IsMain: false},
	})
	g.SetFinder(finder)

	err := g.ScanModules(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(syncer.synced) != 0 {
		t.Errorf("expected 0 synced tables, got %d", len(syncer.synced))
	}
}

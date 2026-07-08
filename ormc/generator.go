
package ormc

import "github.com/tinywasm/model"

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tinywasm/fmt"
)

const (
	tagInput   = "input:\""
	tagDB      = "db:\""
	tagExclude = "-"
)

type FieldInfo struct {
	Name       string
	ColumnName string
	Type       model.FieldType
	PK         bool
	Unique     bool
	NotNull    bool
	AutoInc    bool
	Ref        string
	RefColumn  string
	OnDelete   string
	IsPK       bool
	OldName    string
	GoType     string
	IsPointer  bool // true if the original field is *T (only meaningful for FieldStruct)
	OmitEmpty  bool
	Exclude    bool
	HasDB      bool
	// Permitted config — populated from validate:"..." tag
	Letters           bool
	Tilde             bool
	Numbers           bool
	Spaces            bool
	Extra             []rune
	Minimum           int
	Maximum           int
	WidgetConstructor string   // e.g. "input.Text()"
	Tags              []string // input modifiers e.g. "notilde", "min=2"
}

type StructInfo struct {
	Name              string
	ModelName         string
	PackageName       string
	Fields            []FieldInfo
	ModelNameDeclared bool
	IsForm bool
	NoDB   bool
	SourceFile     string
}

// buildAliasMap scans all .go files in dir and returns a map of
// type alias name → underlying type name (only one-level, primitive aliases).
func buildAliasMap(dir string) map[string]string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	aliases := map[string]string{}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".go" {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			continue
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || !ts.Assign.IsValid() {
					continue
				}
				if ident, ok := ts.Type.(*ast.Ident); ok {
					aliases[ts.Name.Name] = ident.Name
				}
			}
		}
	}
	return aliases
}

// resolveAlias resolves a type name through the alias map (one level).
func resolveAlias(aliases map[string]string, name string) string {
	if aliases == nil {
		return name
	}
	if u, ok := aliases[name]; ok {
		return u
	}
	return name
}

// detectModelName scans the AST for func (X) ModelName() string on structName.
// Returns the literal return value if found, "" otherwise.
func detectModelName(node *ast.File, structName string) string {
	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
			continue
		}
		if funcDecl.Name.Name != "ModelName" {
			continue
		}
		recv := funcDecl.Recv.List[0].Type
		recvName := ""
		if ident, ok := recv.(*ast.Ident); ok {
			recvName = ident.Name
		} else if star, ok := recv.(*ast.StarExpr); ok {
			if ident, ok := star.X.(*ast.Ident); ok {
				recvName = ident.Name
			}
		}
		if recvName != structName {
			continue
		}
		if funcDecl.Body != nil && len(funcDecl.Body.List) == 1 {
			if ret, ok := funcDecl.Body.List[0].(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
				if lit, ok := ret.Results[0].(*ast.BasicLit); ok {
					return fmt.Convert(lit.Value).TrimPrefix(`"`).TrimSuffix(`"`).String()
				}
			}
		}
	}
	return ""
}

// GenerateForStruct reads the Go File and generates the ORM implementations for a given struct name.
func (g *Generator) GenerateForStruct(structName string, goFile string) error {
	infos, err := g.parseDefinitionsInFile(goFile)
	if err != nil {
		return err
	}
	var target *StructInfo
	for _, info := range infos {
		if info.Name == structName {
			target = &info
			break
		}
	}
	if target == nil {
		return fmt.Err("Struct not found in file")
	}
	return g.GenerateForFile([]StructInfo{*target}, goFile)
}

var defaultWidgets = map[string]string{
	"string":  "input.Text()",
	"int":     "input.Number()",
	"int32":   "input.Number()",
	"int64":   "input.Number()",
	"uint":    "input.Number()",
	"uint32":  "input.Number()",
	"uint64":  "input.Number()",
	"float32": "input.Number()",
	"float64": "input.Number()",
	"bool":    "input.Checkbox()",
}

var inputWidgets = map[string]string{
	"text":     "input.Text()",
	"email":    "input.Email()",
	"password": "input.Password()",
	"textarea": "input.Textarea()",
	"phone":    "input.Phone()",
	"number":   "input.Number()",
	"date":     "input.Date()",
	"hour":     "input.Hour()",
	"ip":       "input.IP()",
	"rut":      "input.Rut()",
	"address":  "input.Address()",
	"checkbox": "input.Checkbox()",
	"datalist": "input.Datalist()",
	"select":   "input.Select()",
	"radio":    "input.Radio()",
	"filepath": "input.Filepath()",
	"gender":   "input.Gender()",
}

// tagSetters maps a struct-tag modifier to a wrapper function call.
// %s is replaced with the current widget expression.
// Add an entry here when a new sustractive tag is supported.
// Corresponding helper must exist in tinywasm/form/input (e.g. input.SetTilde).
var tagSetters = map[string]string{
	"notilde": "input.SetTilde(%s, false)",
}

func isModifier(s string) bool {
	return s == "required" || s == "letters" || s == "numbers" || s == "tilde" || s == "notilde" ||
		s == "spaces" || s == "name" || fmt.HasPrefix(s, "min=") || fmt.HasPrefix(s, "max=")
}

func parseInputModifiers(tag string, fi *FieldInfo) {
	parts := fmt.Convert(tag).Split(",")
	for i, v := range parts {
		if i == 0 && !isModifier(v) {
			continue // skip type override
		}
		fi.Tags = append(fi.Tags, v)
		switch {
		case v == "required":
			fi.NotNull = true
		case v == "name":
			fi.Letters = true
			fi.Tilde = true
			fi.Spaces = true
		case v == "letters":
			fi.Letters = true
		case v == "numbers":
			fi.Numbers = true
		case v == "tilde":
			fi.Tilde = true
		case v == "spaces":
			fi.Spaces = true
		case fmt.HasPrefix(v, "min="):
			n, _ := fmt.Convert(v).TrimPrefix("min=").Int64()
			fi.Minimum = int(n)
		case fmt.HasPrefix(v, "max="):
			n, _ := fmt.Convert(v).TrimPrefix("max=").Int64()
			fi.Maximum = int(n)
		}
	}
}

func writePermittedFields(buf *fmt.Conv, f FieldInfo) {
	// Use nested Permitted literal
	hasPerm := f.Letters || f.Tilde || f.Numbers || f.Spaces ||
		len(f.Extra) > 0 || f.Minimum > 0 || f.Maximum > 0

	if !hasPerm {
		return
	}

	buf.Write(", Permitted: model.Permitted{")
	parts := []string{}
	if f.Letters {
		parts = append(parts, "Letters: true")
	}
	if f.Tilde {
		parts = append(parts, "Tilde: true")
	}
	if f.Numbers {
		parts = append(parts, "Numbers: true")
	}
	if f.Spaces {
		parts = append(parts, "Spaces: true")
	}
	if f.Minimum > 0 {
		parts = append(parts, fmt.Sprintf("Minimum: %d", f.Minimum))
	}
	if f.Maximum > 0 {
		parts = append(parts, fmt.Sprintf("Maximum: %d", f.Maximum))
	}
	if len(f.Extra) > 0 {
		buf2 := "Extra: []rune{"
		for i, r := range f.Extra {
			if i > 0 {
				buf2 += ", "
			}
			buf2 += fmt.Sprintf("'%s'", string(r))
		}
		buf2 += "}"
		parts = append(parts, buf2)
	}

	// Join parts
	for i, p := range parts {
		if i > 0 {
			buf.Write(", ")
		}
		buf.Write(p)
	}
	buf.Write("}")
}

// asFields maps FieldInfo to model.Field for sync.
func (s StructInfo) asFields() []model.Field {
	fields := make([]model.Field, len(s.Fields))
	for i, f := range s.Fields {
		fields[i] = model.Field{
			Name:      f.ColumnName,
			Type:      f.Type,
			NotNull:   f.NotNull,
			OmitEmpty: f.OmitEmpty,
			DB: &model.FieldDB{
				PK:      f.PK,
				Unique:  f.Unique,
				AutoInc: f.AutoInc,
			},
		}
	}
	return fields
}

// collectAllStructs walks rootDir and returns a map of all parsed StructInfo
// keyed by struct name. Used by Run() Pass 1.
func (g *Generator) collectAllStructs() (map[string]StructInfo, []string, []string, error) {
	all := make(map[string]StructInfo)
	var structOrder []string
	var fileOrder []string
	fileSeen := make(map[string]bool)

	err := filepath.Walk(g.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			dirName := info.Name()
			if dirName == "vendor" || dirName == ".git" || dirName == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}

		fileName := info.Name()
		if fileName == "model.go" || fileName == "models.go" {
			infos, err := g.parseDefinitionsInFile(path)
			if err != nil {
				return err // Falla ruidoso
			}

			for _, info := range infos {
				all[info.Name] = info
				structOrder = append(structOrder, info.Name)
				if !fileSeen[path] {
					fileSeen[path] = true
					fileOrder = append(fileOrder, path)
				}
			}
		}

		return nil
	})

	return all, structOrder, fileOrder, err
}

// generateAll groups the enriched all map by source file path and calls
// GenerateForFile once per file.
func (g *Generator) generateAll(all map[string]StructInfo, structOrder []string, fileOrder []string) error {
	byFile := make(map[string][]StructInfo)
	for _, structName := range structOrder {
		info := all[structName]
		byFile[info.SourceFile] = append(byFile[info.SourceFile], info)
	}

	for _, sourceFile := range fileOrder {
		infos := byFile[sourceFile]
		if len(infos) > 0 {
			if err := g.GenerateForFile(infos, sourceFile); err != nil {
				g.log(fmt.Sprintf("Failed to write output for %s: %v", sourceFile, err))
			}
		}
	}
	return nil
}

// Run is the entry point for the CLI tool.
func (g *Generator) Run() error {
	// Pass 1: collect all definitions across all model files
	all, structOrder, fileOrder, err := g.collectAllStructs()
	if err != nil {
		return fmt.Err(err, "error walking directory")
	}
	if len(all) == 0 {
		return fmt.Err("no models found")
	}

	// Pass 4: generate (group by source file, call GenerateForFile once per file)
	if err := g.generateAll(all, structOrder, fileOrder); err != nil {
		return err
	}

	// Pass 5: sync dependencies
	if !g.skipTidy {
		if _, err := os.Stat(filepath.Join(g.rootDir, "go.mod")); err == nil {
			g.log("Syncing dependencies...")
			if err := g.exec("go", "mod", "tidy"); err != nil {
				return fmt.Err(err, "failed to tidy module")
			}
		}
	}

	return nil
}

func (g *Generator) exec(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = g.rootDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

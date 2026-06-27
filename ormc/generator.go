
package ormc

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
	Type       fmt.FieldType
	PK         bool
	Unique     bool
	NotNull    bool
	AutoInc    bool
	Ref        string
	RefColumn  string
	IsPK       bool
	OldName    string
	GoType     string
	IsPointer  bool // true if the original field is *T (only meaningful for FieldStruct)
	OmitEmpty  bool
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

// SliceFieldInfo records a slice-of-struct field found in a parent struct.
// Not DB-mapped; used only for relation resolution.
type SliceFieldInfo struct {
	Name     string // e.g. "Roles"
	ElemType string // e.g. "Role"
}

type StructInfo struct {
	Name              string
	ModelName         string
	PackageName       string
	Fields            []FieldInfo
	ModelNameDeclared bool
	IsForm            bool
	HasAnyInputTag    bool // true when ≥1 field has input: tag (including input:"-")
	NoDB              bool
	WantTypedFields   bool
	SourceFile        string
	SliceFields       []SliceFieldInfo // populated by ParseStruct; used by ResolveRelations
	Relations         []RelationInfo   // populated by ResolveRelations; used by GenerateForFile
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

// ParseStruct parses a single struct from a Go file and returns its metadata.
func (g *Generator) ParseStruct(structName string, goFile string) (StructInfo, error) {
	if structName == "" {
		return StructInfo{}, fmt.Err("Please provide a struct name")
	}

	if goFile == "" {
		return StructInfo{}, fmt.Err("goFile path cannot be empty")
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, goFile, nil, parser.ParseComments)
	if err != nil {
		return StructInfo{}, fmt.Err(err, "Failed to parse file")
	}

	var targetStruct *ast.StructType
	var structFound bool
	var wantTypedFields bool

	ast.Inspect(node, func(n ast.Node) bool {
		if genDecl, ok := n.(*ast.GenDecl); ok {
			for _, spec := range genDecl.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					if typeSpec.Name.Name == structName {
						if structType, ok := typeSpec.Type.(*ast.StructType); ok {
							targetStruct = structType
							structFound = true
							if genDecl.Doc != nil {
								for _, comment := range genDecl.Doc.List {
									if fmt.Contains(comment.Text, "orm:typed_fields") {
										wantTypedFields = true
									}
								}
							}
							return false
						}
					}
				}
			}
		}
		return true
	})

	if !structFound {
		return StructInfo{}, fmt.Err("Struct not found in file")
	}

	aliases := buildAliasMap(filepath.Dir(goFile))

	modelName := detectModelName(node, structName)
	declared := modelName != ""
	if !declared {
		modelName = fmt.Convert(structName).SnakeLow().String()
	}

	info := StructInfo{
		Name:              structName,
		ModelName:         modelName,
		PackageName:       node.Name.Name,
		ModelNameDeclared: declared,
		WantTypedFields:   wantTypedFields,
	}

	pkFound := false
	hasForm := false
	hasAnyInputTag := false
	hasDB := false

	type fieldTags struct {
		inputTag    string
		jsonColName string // non-empty if json tag provided a custom field name
	}
	allTags := make([]fieldTags, 0)

	for _, field := range targetStruct.Fields.List {
		if len(field.Names) == 0 {
			continue // Anonymous field, skip for now
		}

		fieldName := field.Names[0].Name
		if !ast.IsExported(fieldName) {
			continue
		}

		dbTag := ""
		jsonTag := ""
		inputTag := ""
		omitEmptyTag := false
		if field.Tag != nil {
			tagVal := fmt.Convert(field.Tag.Value).TrimPrefix("`").TrimSuffix("`").String()
			parts := fmt.Convert(tagVal).Split(" ")
			for _, p := range parts {
				if fmt.HasPrefix(p, tagDB) {
					dbTag = fmt.Convert(p).TrimPrefix(tagDB).TrimSuffix(`"`).String()
				} else if fmt.HasPrefix(p, "json:\"") {
					jsonTag = fmt.Convert(p).TrimPrefix(`json:"`).TrimSuffix(`"`).String()
				} else if fmt.HasPrefix(p, tagInput) {
					inputTag = fmt.Convert(p).TrimPrefix(tagInput).TrimSuffix(`"`).String()
				} else if fmt.HasPrefix(p, "omitempty:\"") {
					v := fmt.Convert(p).TrimPrefix(`omitempty:"`).TrimSuffix(`"`).String()
					omitEmptyTag = (v == "true")
				}
			}
		}

		if dbTag == tagExclude {
			continue
		}

		// Field Type mapping
		var fieldType fmt.FieldType
		var typeStr string
		var isPointer bool

		fType := field.Type
		if star, ok := fType.(*ast.StarExpr); ok {
			isPointer = true
			fType = star.X
		}

		if ident, ok := fType.(*ast.Ident); ok {
			typeStr = ident.Name
		} else if sel, ok := fType.(*ast.SelectorExpr); ok {
			if pkgIdent, ok := sel.X.(*ast.Ident); ok {
				typeStr = pkgIdent.Name + "." + sel.Sel.Name
			}
		} else if arr, ok := fType.(*ast.ArrayType); ok {
			elt := arr.Elt
			isEltPointer := false
			if star, ok := elt.(*ast.StarExpr); ok {
				isEltPointer = true
				elt = star.X
			}

			if eltIdent, ok := elt.(*ast.Ident); ok {
				eltName := resolveAlias(aliases, eltIdent.Name)
				if eltName == "byte" && !isEltPointer {
					typeStr = "[]byte"
				} else {
					prefix := "[]"
					if isEltPointer {
						prefix = "[]*"
					}
					typeStr = prefix + eltName
				}
			}
		}

		typeStr = resolveAlias(aliases, typeStr)

		if typeStr == "time.Time" {
			g.log(fmt.Sprintf("Warning: time.Time not allowed for field %s.%s; use int64+tinywasm/time. Skipping.", structName, fieldName))
			continue
		}

		switch typeStr {
		case "string":
			fieldType = fmt.FieldText
		case "int", "int32", "int64", "uint", "uint32", "uint64":
			fieldType = fmt.FieldInt
		case "float32", "float64":
			fieldType = fmt.FieldFloat
		case "bool":
			fieldType = fmt.FieldBool
		case "[]byte":
			fieldType = fmt.FieldBlob
		case "RawJSON", "fmt.RawJSON":
			fieldType = fmt.FieldRaw
		case "[]int", "[]int32", "[]int64", "[]uint", "[]uint32", "[]uint64":
			fieldType = fmt.FieldIntSlice
		default:
			if fmt.HasPrefix(typeStr, "[]") {
				// Slice of struct (likely)
				fieldType = fmt.FieldStructSlice
				elemType := fmt.Convert(typeStr).TrimPrefix("[]").TrimPrefix("*").String()
				info.SliceFields = append(info.SliceFields, SliceFieldInfo{
					Name:     fieldName,
					ElemType: elemType,
				})
			} else if typeStr != "" && !fmt.Contains(typeStr, "chan ") {
				// If it's a struct (but not time.Time, not slice, not chan), map to FieldStruct
				fieldType = fmt.FieldStruct
			} else {
				g.log(fmt.Sprintf("Warning: unsupported type %s for field %s.%s; skipping. Add db:\"-\" to suppress.", typeStr, structName, fieldName))
				continue
			}
		}

		if isPointer && fieldType != fmt.FieldStruct {
			g.log(fmt.Sprintf("Warning: pointers to primitive types not supported for field %s.%s; skipping. Add db:\"-\" to suppress.", structName, fieldName))
			continue
		}

		colName := fmt.Convert(fieldName).SnakeLow().String()
		jsonColName := ""
		if jsonTag != "" {
			parts := fmt.Convert(jsonTag).Split(",")
			name := parts[0]
			if name == "omitempty" || name == "raw" {
				name = ""
			}
			if name != "" && name != "-" {
				colName = name
				jsonColName = name
			}
		}
		isID, isPK := fmt.IDorPrimaryKey(modelName, fieldName)

		var pk, unique, notNull, autoInc bool
		var ref, refCol, oldName string

		fieldIsPK := false
		if (isID || isPK) && !pkFound {
			fieldIsPK = true
			pkFound = true
			pk = true
			// NOTE: does NOT set hasDB — ID field name alone does not imply DB role.
			// A struct is DB only if at least one field carries a db: tag (value ≠ "-").
		}

		if dbTag != "" {
			hasDB = true
			tagParts := fmt.Convert(dbTag).Split(",")
			for _, p := range tagParts {
				switch {
				case p == "pk":
					if !fieldIsPK {
						pk = true
						fieldIsPK = true
						pkFound = true
					}
				case p == "unique":
					unique = true
				case p == "not_null":
					notNull = true
				case p == "autoinc" || p == "autoincrement":
					if fieldType == fmt.FieldText {
						return StructInfo{}, fmt.Err("autoincrement not allowed on FieldText")
					}
					autoInc = true
				case fmt.HasPrefix(p, "ref="):
					refVal := fmt.Convert(p).TrimPrefix("ref=").String()
					refParts := fmt.Convert(refVal).Split(":")
					ref = refParts[0]
					if len(refParts) > 1 {
						refCol = refParts[1]
					}
				case fmt.HasPrefix(p, "old_name="):
					oldName = fmt.Convert(p).TrimPrefix("old_name=").String()
				}
			}
		}

		omitEmpty := omitEmptyTag
		if jsonTag != "" {
			parts := fmt.Convert(jsonTag).Split(",")
			for _, p := range parts {
				if p == "omitempty" {
					omitEmpty = true
				}
				if p == "raw" {
					fieldType = fmt.FieldRaw
				}
			}
		}

		fi := FieldInfo{
			Name:       fieldName,
			ColumnName: colName,
			Type:       fieldType,
			PK:         pk,
			Unique:     unique,
			NotNull:    notNull,
			AutoInc:    autoInc,
			Ref:        ref,
			RefColumn:  refCol,
			IsPK:       fieldIsPK,
			OldName:    oldName,
			GoType:     typeStr,
			IsPointer:  isPointer,
			OmitEmpty:  omitEmpty,
		}

		if inputTag != "" {
			hasAnyInputTag = true
			if inputTag != tagExclude {
				hasForm = true
			}
		}

		info.Fields = append(info.Fields, fi)
		allTags = append(allTags, fieldTags{inputTag: inputTag, jsonColName: jsonColName})
	}

	if len(info.Fields) == 0 {
		g.log(fmt.Sprintf("Warning: struct %s skipped (no serializable fields)", info.Name))
		return StructInfo{}, nil
	}

	info.IsForm = hasForm
	info.HasAnyInputTag = hasAnyInputTag
	info.NoDB = !hasDB

	if !info.NoDB {
		for i, fi := range info.Fields {
			if allTags[i].jsonColName != "" {
				return StructInfo{}, fmt.Err(fmt.Sprintf(
					"field %s.%s: json:\"%s\" is not allowed on DB structs — column name is always derived as snake_case (%s)",
					structName, fi.Name, allTags[i].jsonColName, fmt.Convert(fi.Name).SnakeLow().String(),
				))
			}
		}
	}

	if info.NoDB && info.WantTypedFields {
		g.log(fmt.Sprintf("Warning: orm:typed_fields ignored on codec-only struct %s", info.Name))
		info.WantTypedFields = false
	}

	// Second pass for widgets/modifiers now that info.IsForm is known
	for i := range info.Fields {
		fi := &info.Fields[i]
		inputTag := allTags[i].inputTag

		if info.IsForm {
			if inputTag == tagExclude {
				// No widget
			} else if inputTag != "" {
				typeName := fmt.Convert(inputTag).Split(",")[0]
				if !isModifier(typeName) {
					if ctor, ok := inputWidgets[typeName]; ok {
						fi.WidgetConstructor = ctor
					} else {
						g.log("Warning: unknown input type", typeName, "for field", fi.Name)
						if ctor, ok := defaultWidgets[fi.GoType]; ok {
							fi.WidgetConstructor = ctor
						}
					}
				} else {
					if ctor, ok := defaultWidgets[fi.GoType]; ok {
						fi.WidgetConstructor = ctor
					}
				}
				parseInputModifiers(inputTag, fi)
			} else {
				if ctor, ok := defaultWidgets[fi.GoType]; ok {
					fi.WidgetConstructor = ctor
				}
			}
		} else if inputTag != "" && inputTag != tagExclude {
			// Struct without form role (no input: tag with widget), but with input: tag for modifiers
			parseInputModifiers(inputTag, fi)
		}
	}

	return info, nil
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

// GenerateForStruct reads the Go File and generates the ORM implementations for a given struct name.
func (g *Generator) GenerateForStruct(structName string, goFile string) error {
	info, err := g.ParseStruct(structName, goFile)
	if err != nil {
		return err
	}
	if len(info.Fields) == 0 {
		return nil
	}
	return g.GenerateForFile([]StructInfo{info}, goFile)
}

func writePermittedFields(buf *fmt.Conv, f FieldInfo) {
	// Use nested Permitted literal
	hasPerm := f.Letters || f.Tilde || f.Numbers || f.Spaces ||
		len(f.Extra) > 0 || f.Minimum > 0 || f.Maximum > 0

	if !hasPerm {
		return
	}

	buf.Write(", Permitted: fmt.Permitted{")
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

// parseStructsInFile parses all structs in a given file.
func (g *Generator) parseStructsInFile(path string) ([]StructInfo, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var infos []StructInfo
	for _, decl := range node.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
			for _, spec := range genDecl.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					if _, ok := typeSpec.Type.(*ast.StructType); ok {
						info, err := g.ParseStruct(typeSpec.Name.Name, path)
						if err != nil {
							g.log(fmt.Sprintf("Skipping %s in %s: %v", typeSpec.Name.Name, path, err))
							continue
						}
						if len(info.Fields) == 0 {
							// Already logged in ParseStruct
							continue
						}
						info.SourceFile = path
						infos = append(infos, info)
					}
				}
			}
		}
	}
	return infos, nil
}

// asFields maps FieldInfo to fmt.Field for sync.
func (s StructInfo) asFields() []fmt.Field {
	fields := make([]fmt.Field, len(s.Fields))
	for i, f := range s.Fields {
		fields[i] = fmt.Field{
			Name:      f.ColumnName,
			Type:      f.Type,
			NotNull:   f.NotNull,
			OmitEmpty: f.OmitEmpty,
			DB: &fmt.FieldDB{
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
			infos, err := g.parseStructsInFile(path)
			if err != nil {
				return nil // Skip unparseable files
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
	// Pass 1: collect all structs across all model files (BEFORE cleanup)
	all, structOrder, fileOrder, err := g.collectAllStructs()
	if err != nil {
		return fmt.Err(err, "error walking directory")
	}
	if len(all) == 0 {
		return fmt.Err("no models found")
	}

	// Pass 2: cleanup tags (Pass 1 metadata is already safe)
	for _, f := range fileOrder {
		if err := g.RewriteModelTags(f); err != nil {
			g.log(fmt.Sprintf("Warning: failed to rewrite tags in %s: %v", f, err))
		}
	}

	// Pass 3: resolve cross-struct relations
	g.ResolveRelations(all)

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

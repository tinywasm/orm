package ormc

import "github.com/tinywasm/model"

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

)

var fieldTypeByName = map[string]model.FieldType{
	"FieldText":   model.FieldText,
	"FieldInt":    model.FieldInt,
	"FieldFloat":  model.FieldFloat,
	"FieldBool":   model.FieldBool,
	"FieldBlob":   model.FieldBlob,
	"FieldStruct": model.FieldStruct,
	"FieldRaw":    model.FieldRaw,
}

// parseGenerated extracts (modelName → []model.Field) from a generated *_orm.go
// file by reading its `func (m *T) ModelName() string { return "<table>" }` and
// its `var _schema<T> = []model.Field{ … }` composite literal.
func parseGenerated(path string) (map[string][]model.Field, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	// dbStructs collects structs that have ReadOne*/ReadAll* helpers — the
	// marker that ormc emitted DB helpers (i.e. the struct is NOT orm:no_db).
	// Structs without these helpers must not be synced even if they have ModelName().
	// Note: old generated files (pre orm:no_db) may lack ReadOne/ReadAll but still
	// be valid DB structs — we fall back to allowing all structs if none are found,
	// preserving backward compatibility with cached module generated files.
	dbStructs := make(map[string]bool) // structName -> is DB struct
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue // only package-level funcs
		}
		name := fn.Name.Name
		if strings.HasPrefix(name, "ReadOne") || strings.HasPrefix(name, "ReadAll") {
			structName := strings.TrimPrefix(strings.TrimPrefix(name, "ReadOne"), "ReadAll")
			dbStructs[structName] = true
		}
	}
	// If no ReadOne/ReadAll found, file predates orm:no_db — treat all structs as DB.
	allAreDB := len(dbStructs) == 0

	modelNames := make(map[string]string) // structName -> tableName
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 || fn.Name.Name != "ModelName" {
			continue
		}
		var structName string
		recv := fn.Recv.List[0].Type
		if ident, ok := recv.(*ast.Ident); ok {
			structName = ident.Name
		} else if star, ok := recv.(*ast.StarExpr); ok {
			if ident, ok := star.X.(*ast.Ident); ok {
				structName = ident.Name
			}
		}
		if structName == "" {
			continue
		}

		if fn.Body != nil && len(fn.Body.List) == 1 {
			if ret, ok := fn.Body.List[0].(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
				if lit, ok := ret.Results[0].(*ast.BasicLit); ok {
					modelNames[structName] = strings.Trim(lit.Value, "\"")
				}
			}
		}
	}

	schemas := make(map[string][]model.Field)
	for _, decl := range node.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vspec, ok := spec.(*ast.ValueSpec)
			if !ok || len(vspec.Names) == 0 || !strings.HasPrefix(vspec.Names[0].Name, "_schema") {
				continue
			}
			structName := strings.TrimPrefix(vspec.Names[0].Name, "_schema")
			tableName, ok := modelNames[structName]
			if !ok {
				continue
			}
			// Skip structs without DB helpers — they were generated with orm:no_db.
			// ModelName() is always emitted (used by form/API layers), but only
			// structs with ReadOne*/ReadAll* have an actual DB table to sync.
			if !allAreDB && !dbStructs[structName] {
				continue
			}

			if len(vspec.Values) == 0 {
				continue
			}
			lit, ok := vspec.Values[0].(*ast.CompositeLit)
			if !ok {
				continue
			}

			var fields []model.Field
			for _, elt := range lit.Elts {
				fieldLit, ok := elt.(*ast.CompositeLit)
				if !ok {
					continue
				}
				var field model.Field
				for _, kv := range fieldLit.Elts {
					kve, ok := kv.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key, ok := kve.Key.(*ast.Ident)
					if !ok {
						continue
					}

					switch key.Name {
					case "Name":
						if blit, ok := kve.Value.(*ast.BasicLit); ok {
							field.Name = strings.Trim(blit.Value, "\"")
						}
					case "Type":
						typeName := ""
						if ident, ok := kve.Value.(*ast.Ident); ok {
							typeName = ident.Name
						} else if sel, ok := kve.Value.(*ast.SelectorExpr); ok {
							typeName = sel.Sel.Name
						}
						if t, ok := fieldTypeByName[typeName]; ok {
							field.Type = t
						}
					case "NotNull":
						if ident, ok := kve.Value.(*ast.Ident); ok && ident.Name == "true" {
							field.NotNull = true
						}
					case "OmitEmpty":
						if ident, ok := kve.Value.(*ast.Ident); ok && ident.Name == "true" {
							field.OmitEmpty = true
						}
					case "DB":
						field.DB = parseFieldDB(kve.Value)
					}
				}
				fields = append(fields, field)
			}
			schemas[tableName] = fields
		}
	}

	return schemas, nil
}

func parseFieldDB(expr ast.Expr) *model.FieldDB {
	// Expecting &model.FieldDB{...}
	unary, ok := expr.(*ast.UnaryExpr)
	if !ok || unary.Op != token.AND {
		return nil
	}
	lit, ok := unary.X.(*ast.CompositeLit)
	if !ok {
		return nil
	}

	db := &model.FieldDB{}
	for _, elt := range lit.Elts {
		kve, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kve.Key.(*ast.Ident)
		if !ok {
			continue
		}
		valIdent, ok := kve.Value.(*ast.Ident)
		if !ok || valIdent.Name != "true" {
			continue
		}

		switch key.Name {
		case "PK":
			db.PK = true
		case "Unique":
			db.Unique = true
		case "AutoInc":
			db.AutoInc = true
		}
	}
	return db
}

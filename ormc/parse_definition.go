
package ormc

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"

	"github.com/tinywasm/fmt"
	"github.com/tinywasm/model"
)

func (g *Generator) parseDefinitionsInFile(path string) ([]StructInfo, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var infos []StructInfo
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			continue
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for i, name := range valueSpec.Names {
				if !strings.HasSuffix(name.Name, "Model") {
					// Plan §4.2: Falla ruidoso if it doesn't end in Model but looks like a Definition?
					// For now, only process if it ends in Model as per §4.2.
					continue
				}

				if i >= len(valueSpec.Values) {
					continue
				}

				info, err := g.parseDefinition(name.Name, valueSpec.Values[i], node)
				if err != nil {
					return nil, err // Falla ruidoso
				}
				if info != nil {
					info.SourceFile = path
					info.PackageName = node.Name.Name
					infos = append(infos, *info)
				}
			}
		}
	}
	return infos, nil
}

func (g *Generator) parseDefinition(varName string, expr ast.Expr, file *ast.File) (*StructInfo, error) {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, nil
	}

	// Check if type is model.Definition
	typeStr := exprToString(cl.Type)
	if !strings.Contains(typeStr, "Definition") {
		return nil, nil
	}

	structName := strings.TrimSuffix(varName, "Model")
	if structName == "" {
		return nil, fmt.Errf("Variable name %s must have a prefix before 'Model'", varName)
	}

	info := &StructInfo{
		Name:              structName,
		ModelNameDeclared: true,
	}

	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		key := exprToString(kv.Key)
		switch key {
		case "Name":
			if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				val, _ := strconv.Unquote(lit.Value)
				info.ModelName = val
			}
		case "Fields":
			fields, err := g.parseFields(kv.Value)
			if err != nil {
				return nil, err
			}
			info.Fields = fields
		}
	}

	if info.ModelName == "" {
		info.ModelName = ToSnakeCase(info.Name)
	}

	// Determine NoDB, IsForm, etc.
	hasDB := false
	hasForm := false
	for _, f := range info.Fields {
		if f.HasDB {
			hasDB = true
		}
		if f.WidgetConstructor != "" {
			hasForm = true
		}
	}
	info.NoDB = !hasDB
	info.IsForm = hasForm

	return info, nil
}

func (g *Generator) parseFields(expr ast.Expr) ([]FieldInfo, error) {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil, nil
	}

	var fields []FieldInfo
	for _, elt := range cl.Elts {
		fieldCl, ok := elt.(*ast.CompositeLit)
		if !ok {
			continue
		}

		fi, err := g.parseField(fieldCl)
		if err != nil {
			return nil, err
		}
		fields = append(fields, fi)
	}
	return fields, nil
}

func (g *Generator) parseField(cl *ast.CompositeLit) (FieldInfo, error) {
	var fi FieldInfo
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		key := exprToString(kv.Key)
		switch key {
		case "Name":
			if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				fi.ColumnName, _ = strconv.Unquote(lit.Value)
				fi.Name = fmt.Convert(fi.ColumnName).CamelUp().String()
			}
		case "Type":
			fi.Type = parseFieldType(kv.Value)
		case "NotNull":
			fi.NotNull = exprToBool(kv.Value)
		case "OmitEmpty":
			fi.OmitEmpty = exprToBool(kv.Value)
		case "Exclude":
			fi.Exclude = exprToBool(kv.Value)
		case "Widget":
			fi.WidgetConstructor = exprToString(kv.Value)
		case "DB":
			g.parseDBField(kv.Value, &fi)
		case "Ref":
			fi.Ref = parseRef(kv.Value)
		case "Permitted":
			g.parsePermitted(kv.Value, &fi)
		}
	}

	// Plan §4.4: Falla ruidoso if Ref is nil for FieldStruct/FieldStructSlice
	if (fi.Type == model.FieldStruct || fi.Type == model.FieldStructSlice) && fi.Ref == "" {
		return FieldInfo{}, fmt.Errf("Field %s: Ref must not be nil for FieldStruct/FieldStructSlice", fi.ColumnName)
	}

	// Post-process type mapping to GoType
	fi.GoType = FieldTypeToGoType(fi.Type, fi.Ref)

	return fi, nil
}

func (g *Generator) parseDBField(expr ast.Expr, fi *FieldInfo) {
	fi.HasDB = true
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op == token.AND {
			if cl2, ok := unary.X.(*ast.CompositeLit); ok {
				cl = cl2
			}
		}
	}

	if cl == nil {
		return
	}

	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key := exprToString(kv.Key)
		switch key {
		case "PK":
			fi.PK = exprToBool(kv.Value)
		case "Unique":
			fi.Unique = exprToBool(kv.Value)
		case "AutoInc":
			fi.AutoInc = exprToBool(kv.Value)
		case "RefColumn":
			if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				fi.RefColumn, _ = strconv.Unquote(lit.Value)
			}
		case "OnDelete":
			if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				fi.OnDelete, _ = strconv.Unquote(lit.Value)
			}
		}
	}
}

func (g *Generator) parsePermitted(expr ast.Expr, fi *FieldInfo) {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op == token.AND {
			if cl2, ok := unary.X.(*ast.CompositeLit); ok {
				cl = cl2
			}
		}
	}
	if cl == nil {
		return
	}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key := exprToString(kv.Key)
		switch key {
		case "Letters":
			fi.Letters = exprToBool(kv.Value)
		case "Tilde":
			fi.Tilde = exprToBool(kv.Value)
		case "Numbers":
			fi.Numbers = exprToBool(kv.Value)
		case "Spaces":
			fi.Spaces = exprToBool(kv.Value)
		case "Minimum":
			fi.Minimum = exprToInt(kv.Value)
		case "Maximum":
			fi.Maximum = exprToInt(kv.Value)
		}
	}
}

func parseFieldType(expr ast.Expr) model.FieldType {
	s := exprToString(expr)
	switch {
	case strings.Contains(s, "FieldText"):
		return model.FieldText
	case strings.Contains(s, "FieldIntSlice"):
		return model.FieldIntSlice
	case strings.Contains(s, "FieldInt"):
		return model.FieldInt
	case strings.Contains(s, "FieldFloat"):
		return model.FieldFloat
	case strings.Contains(s, "FieldBool"):
		return model.FieldBool
	case strings.Contains(s, "FieldBlob"):
		return model.FieldBlob
	case strings.Contains(s, "FieldStructSlice"):
		return model.FieldStructSlice
	case strings.Contains(s, "FieldStruct"):
		return model.FieldStruct
	case strings.Contains(s, "FieldRaw"):
		return model.FieldRaw
	}
	return model.FieldText
}

func parseRef(expr ast.Expr) string {
	s := exprToString(expr)
	s = strings.TrimPrefix(s, "&")
	return s
}

func exprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	case *ast.BasicLit:
		return t.Value
	case *ast.UnaryExpr:
		return t.Op.String() + exprToString(t.X)
	case *ast.CallExpr:
		s := exprToString(t.Fun) + "("
		for i, arg := range t.Args {
			if i > 0 {
				s += ", "
			}
			s += exprToString(arg)
		}
		s += ")"
		return s
	case *ast.CompositeLit:
		s := ""
		if t.Type != nil {
			s += exprToString(t.Type)
		}
		s += "{"
		for i, elt := range t.Elts {
			if i > 0 {
				s += ", "
			}
			s += exprToString(elt)
		}
		s += "}"
		return s
	case *ast.KeyValueExpr:
		return exprToString(t.Key) + ": " + exprToString(t.Value)
	}
	return ""
}

func exprToBool(expr ast.Expr) bool {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name == "true"
	}
	return false
}

func exprToInt(expr ast.Expr) int {
	if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.INT {
		val, _ := strconv.Atoi(lit.Value)
		return val
	}
	return 0
}

func ToSnakeCase(s string) string {
	return fmt.Convert(s).SnakeLow().String()
}


func FieldTypeToGoType(ft model.FieldType, ref string) string {
	switch ft {
	case model.FieldText, model.FieldRaw:
		return "string"
	case model.FieldInt:
		return "int64"
	case model.FieldFloat:
		return "float64"
	case model.FieldBool:
		return "bool"
	case model.FieldBlob:
		return "[]byte"
	case model.FieldIntSlice:
		return "[]int"
	case model.FieldStruct:
		if ref != "" {
			return resolveStructType(ref)
		}
		return "struct{}"
	case model.FieldStructSlice:
		if ref != "" {
			return "[]" + resolveStructType(ref)
		}
		return "[]struct{}"
	}
	return "string"
}

func resolveStructType(ref string) string {
	// ref is like "UserModel" or "pkg.UserModel"
	parts := strings.Split(ref, ".")
	last := parts[len(parts)-1]
	if strings.HasSuffix(last, "Model") {
		parts[len(parts)-1] = strings.TrimSuffix(last, "Model")
	}
	return strings.Join(parts, ".")
}

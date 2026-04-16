//go:build !wasm

package orm

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"

	"github.com/tinywasm/fmt"
)

// RewriteModelTags performs in-place struct tag cleanup in the given file.
// It removes 'form' and 'validate' tags, and strips field names from 'json' tags.
// For ormc:formonly structs, json field names are preserved (needed for camelCase protocols).
func (o *Ormc) RewriteModelTags(path string) error {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	type edit struct {
		start, end int
		newVal     string
	}
	var edits []edit

	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}

		formOnly := false
		if genDecl.Doc != nil {
			for _, c := range genDecl.Doc.List {
				if fmt.Contains(c.Text, "ormc:formonly") {
					formOnly = true
					break
				}
			}
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			for _, field := range structType.Fields.List {
				if field.Tag == nil {
					continue
				}
				tagValue := field.Tag.Value
				if !fmt.HasPrefix(tagValue, "`") || !fmt.HasSuffix(tagValue, "`") {
					continue
				}

				rawTag := fmt.Convert(tagValue).TrimPrefix("`").TrimSuffix("`").String()
				newTag := rewriteRawTag(rawTag, formOnly)
				if newTag == rawTag {
					continue
				}
				edits = append(edits, edit{
					start:  int(field.Tag.Pos()),
					end:    int(field.Tag.End()),
					newVal: "`" + newTag + "`",
				})
			}
		}
	}

	if len(edits) == 0 {
		return nil
	}

	// Apply edits in reverse to keep offsets valid
	result := content
	for i := len(edits) - 1; i >= 0; i-- {
		e := edits[i]
		start := fset.Position(token.Pos(e.start)).Offset
		end := fset.Position(token.Pos(e.end)).Offset

		var buf bytes.Buffer
		buf.Write(result[:start])
		buf.WriteString(e.newVal)
		buf.Write(result[end:])
		result = buf.Bytes()
	}

	return os.WriteFile(path, result, 0644)
}

// rewriteRawTag cleans a raw struct tag string.
// For non-formonly structs: removes form/validate tags and strips json field names (keeps only omitempty).
// For formonly structs: removes form/validate tags but preserves json field names (needed for camelCase protocols).
func rewriteRawTag(raw string, formOnly bool) string {
	parts := fmt.Convert(raw).Split(" ")
	var kept []string

	for _, p := range parts {
		if p == "" {
			continue
		}
		if fmt.HasPrefix(p, "form:") || fmt.HasPrefix(p, "validate:") {
			continue
		}
		if fmt.HasPrefix(p, "json:\"") {
			val := fmt.Convert(p).TrimSuffix("\"").TrimPrefix("json:\"").String()
			if val == "-" {
				kept = append(kept, p)
				continue
			}

			subParts := fmt.Convert(val).Split(",")
			name := subParts[0]
			hasOmit := false
			hasRaw := false
			for _, sp := range subParts {
				if sp == "omitempty" {
					hasOmit = true
				}
				if sp == "raw" {
					hasRaw = true
				}
			}

			if formOnly && name != "" {
				// preserve json name for formonly structs
				tagVal := name
				if hasOmit {
					tagVal += ",omitempty"
				}
				if hasRaw {
					tagVal += ",raw"
				}
				kept = append(kept, fmt.Sprintf("json:\"%s\"", tagVal))
			} else {
				tagVal := ""
				if hasOmit {
					tagVal += ",omitempty"
				}
				if hasRaw {
					tagVal += ",raw"
				}
				if tagVal != "" {
					kept = append(kept, fmt.Sprintf("json:\"%s\"", tagVal))
				}
			}
			// non-formonly without omitempty: drop the json tag entirely
			continue
		}
		kept = append(kept, p)
	}

	return fmt.Convert(kept).Join(" ").String()
}

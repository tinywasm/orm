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

		noDB := false
		if genDecl.Doc != nil {
			for _, c := range genDecl.Doc.List {
				if fmt.Contains(c.Text, "orm:no_db") {
					noDB = true
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
				newTag := rewriteRawTag(rawTag, noDB)
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
// For non-no_db structs: removes form/validate tags and strips json field names (keeps only omitempty).
// For no_db structs: removes form/validate tags but preserves json field names (needed for camelCase protocols).
func rewriteRawTag(raw string, noDB bool) string {
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
			if name == "omitempty" {
				hasOmit = true
				name = ""
			} else if name == "raw" {
				name = ""
			}
			for i, sp := range subParts {
				if i == 0 && (hasOmit || sp == "raw") {
					continue
				}
				if sp == "omitempty" {
					hasOmit = true
				}
			}

			if noDB {
				// preserve json name for no_db structs
				var parts []string
				parts = append(parts, name)
				if hasOmit {
					parts = append(parts, "omitempty")
				}

				if len(parts) > 1 || (len(parts) == 1 && parts[0] != "") {
					// Reconstruct the tag. If name was empty but we have options,
					// it will correctly start with a comma (e.g., ",omitempty").
					kept = append(kept, fmt.Sprintf("json:\"%s\"", fmt.Convert(parts).Join(",").String()))
				}
			} else {
				// DB field without explicit json name: only preserve if there's a name
				// omitempty without name adds nothing — the ORM derives the name from the field
				// In this context, the json tag is completely discarded
			}
			// non-formonly without omitempty: drop the json tag entirely
			continue
		}
		kept = append(kept, p)
	}

	return fmt.Convert(kept).Join(" ").String()
}

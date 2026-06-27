package ddl

import (
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
)

// TopologicalSort returns models sorted so parents come before children (Kahn's BFS).
// Models not implementing SchemaExt() are treated as having no FK deps.
// Returns error on circular FK dependency.
func TopologicalSort(models []fmt.Model) ([]fmt.Model, error) {
	byName := make(map[string]fmt.Model, len(models))
	rdeps := make(map[string][]string)
	inDeg := make(map[string]int, len(models))

	for _, m := range models {
		name := m.ModelName()
		byName[name] = m
		if _, ok := inDeg[name]; !ok {
			inDeg[name] = 0
		}
		if ext, ok := m.(interface{ SchemaExt() []orm.FieldExt }); ok {
			for _, f := range ext.SchemaExt() {
				if f.Ref != "" {
					rdeps[f.Ref] = append(rdeps[f.Ref], name)
					inDeg[name]++
				}
			}
		}
	}

	var queue []string
	for _, m := range models {
		if inDeg[m.ModelName()] == 0 {
			queue = append(queue, m.ModelName())
		}
	}

	result := make([]fmt.Model, 0, len(models))
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		result = append(result, byName[name])
		for _, dep := range rdeps[name] {
			inDeg[dep]--
			if inDeg[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(result) != len(models) {
		return nil, fmt.Err("ddl: circular FK dependency detected")
	}
	return result, nil
}

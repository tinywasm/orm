package orm

import . "github.com/tinywasm/fmt"

// ScanAny maps a JSON-decoded Go value (any) into a typed pointer.
// Used by host-side adapters (REST, SQLite driver) where values come
// from json.Unmarshal rather than from js.Value (see jsvalue.ScanValue).
func ScanAny(v any, dest any) error {
	switch p := dest.(type) {
	case *string:
		if s, ok := v.(string); ok {
			*p = s
		}
	case *int:
		switch n := v.(type) {
		case float64:
			*p = int(n)
		case int64:
			*p = int(n)
		}
	case *int64:
		switch n := v.(type) {
		case float64:
			*p = int64(n)
		case int64:
			*p = n
		}
	case *float64:
		if n, ok := v.(float64); ok {
			*p = n
		}
	case *bool:
		if b, ok := v.(bool); ok {
			*p = b
		}
	case *[]byte:
		switch b := v.(type) {
		case []byte:
			*p = b
		case string:
			*p = []byte(b)
		}
	case *any:
		*p = v
	default:
		return Errf("orm: unsupported scan type: %T", dest)
	}
	return nil
}

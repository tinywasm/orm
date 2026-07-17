package orm

import (
	"github.com/tinywasm/fmt"
)

// Factory builds a ready *DB from a DSN. It matches the executor adapters'
// existing constructors.
type Factory func(dsn string) (*DB, error)

// factoryEntry pairs a scheme with its Factory. registry is a plain slice, scanned
// linearly: Go maps are prohibited in tinywasm (TinyGo's map runtime is heavy and
// bloats the wasm binary) — see AGENTS.md. The registry only ever holds a handful of
// entries (one per imported adapter), so a linear scan costs nothing in practice.
type factoryEntry struct {
	scheme  string
	factory Factory
}

var registry []factoryEntry

// Register binds a URL scheme (e.g. "postgres", "sqlite") to a Factory.
// Adapters call this from init(). Last registration for a scheme wins.
func Register(scheme string, f Factory) {
	for i, e := range registry {
		if e.scheme == scheme {
			registry[i].factory = f
			return
		}
	}
	registry = append(registry, factoryEntry{scheme: scheme, factory: f})
}

// Open parses the scheme of dsn, looks up the registered Factory, and
// returns a ready *DB. Errors if the scheme is unknown (adapter not imported).
func Open(dsn string) (*DB, error) {
	scheme := ""
	if fmt.Contains(dsn, "://") {
		scheme = fmt.Convert(dsn).Split("://")[0]
	} else if fmt.HasPrefix(dsn, "sqlite:") {
		scheme = "sqlite"
	}

	if scheme == "" {
		return nil, fmt.Err("orm.Open: could not detect scheme in DSN:", dsn)
	}

	for _, e := range registry {
		if e.scheme == scheme {
			return e.factory(dsn)
		}
	}
	return nil, fmt.Err("orm.Open: unknown scheme", scheme, "(did you forget to import the adapter?)")
}

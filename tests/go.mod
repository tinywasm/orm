// Separate test module: isolates the form-codegen test deps (form → dom, html)
// so the root github.com/tinywasm/orm module stays fmt + modfind only.
module github.com/tinywasm/orm/tests

go 1.25.2

require (
	github.com/tinywasm/fmt v0.24.0
	github.com/tinywasm/form v0.2.6
	github.com/tinywasm/orm v0.0.0
)

require (
	github.com/tinywasm/dom v0.10.1 // indirect
	github.com/tinywasm/html v0.0.2 // indirect
	github.com/tinywasm/modfind v0.0.4 // indirect
)


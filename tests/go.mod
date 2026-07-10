// Separate test module: isolates the form-codegen test deps (form → dom, html)
// so the root github.com/tinywasm/orm module stays fmt + modfind only.
module github.com/tinywasm/orm/tests

go 1.25.2

require (
	github.com/tinywasm/ddlc v0.0.1
	github.com/tinywasm/fmt v0.25.1
	github.com/tinywasm/model v0.0.7
	github.com/tinywasm/orm v0.9.26
)

replace github.com/tinywasm/orm => ..

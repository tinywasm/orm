package ormcp

type QueryArgs struct {
	SQL string `json:"SQL" input:"required"`
}

type ExecArgs struct {
	SQL string `json:"SQL" input:"required"`
}

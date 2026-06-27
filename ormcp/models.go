package ormcp

type QueryArgs struct {
	SQL string `json:"SQL"`
}

type ExecArgs struct {
	SQL string `json:"SQL"`
}

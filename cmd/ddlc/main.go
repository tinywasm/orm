package main

import (
	"flag"
	"os"

	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm/ddl"
	"github.com/tinywasm/orm/ormc"
	"github.com/tinywasm/postgres"
	"github.com/tinywasm/sqlt"
)

var (
	rootFlag    = flag.String("root", ".", "Directory to scan for model.go / models.go")
	outFlag     = flag.String("out", "-", "Output file. Use \"-\" for stdout.")
	dialectFlag = flag.String("dialect", "sqlite", "SQL dialect: sqlite | postgres")
)

func main() {
	flag.Parse()
	var exporter ddl.Exporter
	switch *dialectFlag {
	case "postgres":
		exporter = postgres.NewCompiler()
	default:
		exporter = sqlt.NewCompiler()
	}
	g := ormc.New()
	sql, err := g.ExportSQL(*rootFlag, exporter)
	if err != nil {
		fmt.Println("ddlc:", err)
		os.Exit(1)
	}
	if *outFlag == "-" {
		fmt.Print(sql)
		return
	}
	if err := os.WriteFile(*outFlag, []byte(sql), 0644); err != nil {
		fmt.Println("ddlc:", err)
		os.Exit(1)
	}
}

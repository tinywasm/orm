//go:build !wasm

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/tinywasm/orm"
)

func main() {
	fields := flag.Bool("fields", false, "generate field descriptor variables (e.g. User_.Name)")
	flag.Parse()

	o := orm.NewOrmc()
	o.SetFields(*fields)
	o.SetLog(func(messages ...any) {
		fmt.Fprintln(os.Stderr, messages...)
	})
	if err := o.Run(); err != nil {
		log.Fatalf("ormc: %v", err)
	}
}

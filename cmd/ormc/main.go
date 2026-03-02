//go:build !wasm

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/tinywasm/orm"
)

func main() {
	o := orm.NewOrmc()
	o.SetLog(func(messages ...any) {
		fmt.Fprintln(os.Stderr, messages...)
	})
	if err := o.Run(); err != nil {
		log.Fatalf("ormc: %v", err)
	}
}

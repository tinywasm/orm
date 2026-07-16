//go:build !wasm

package tests

import (
	"testing"
)

func TestCoreLogic_Stlib(t *testing.T) {
	RunCoreTests(t)
}

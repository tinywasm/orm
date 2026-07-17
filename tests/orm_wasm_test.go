//go:build wasm

package tests

import (
	"testing"
)

func TestCoreLogic_Wasm(t *testing.T) {
	RunCoreTests(t)
}

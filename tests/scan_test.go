package tests

import (
	"testing"

	"github.com/tinywasm/orm"
)

func TestScanAny(t *testing.T) {
	// string
	var s string
	if err := orm.ScanAny("hello", &s); err != nil || s != "hello" {
		t.Fatalf("string: got %q err %v", s, err)
	}

	// int from float64 (JSON default)
	var i int
	if err := orm.ScanAny(float64(42), &i); err != nil || i != 42 {
		t.Fatalf("int from float64: got %d err %v", i, err)
	}

	// int64 from float64
	var i64 int64
	if err := orm.ScanAny(float64(99), &i64); err != nil || i64 != 99 {
		t.Fatalf("int64 from float64: got %d err %v", i64, err)
	}

	// int64 from int64
	if err := orm.ScanAny(int64(7), &i64); err != nil || i64 != 7 {
		t.Fatalf("int64 from int64: got %d err %v", i64, err)
	}

	// float64
	var f float64
	if err := orm.ScanAny(3.14, &f); err != nil || f != 3.14 {
		t.Fatalf("float64: got %f err %v", f, err)
	}

	// bool
	var b bool
	if err := orm.ScanAny(true, &b); err != nil || !b {
		t.Fatalf("bool: got %v err %v", b, err)
	}

	// []byte from string
	var bs []byte
	if err := orm.ScanAny("data", &bs); err != nil || string(bs) != "data" {
		t.Fatalf("[]byte from string: got %q err %v", bs, err)
	}

	// any
	var a any
	if err := orm.ScanAny("x", &a); err != nil || a != "x" {
		t.Fatalf("any: got %v err %v", a, err)
	}

	// unsupported type → error
	var ch chan int
	if err := orm.ScanAny("x", &ch); err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

package common

import (
	"bytes"
	"testing"
	"time"
)

type sample struct {
	A int
	B string
}

func TestIsNil(t *testing.T) {
	// plain nil
	if !IsNil(nil) {
		t.Fatal("nil should be nil")
	}

	// typed nil pointer in interface
	var buf *bytes.Buffer = nil
	var i any = buf
	if !IsNil(i) {
		t.Fatal("typed nil pointer held in interface should be nil")
	}

	// non-nil pointer
	b := &bytes.Buffer{}
	if IsNil(b) {
		t.Fatal("non-nil pointer should not be nil")
	}

	// slices/maps
	var s []int
	if !IsNil(s) {
		t.Fatal("nil slice should be nil")
	}
	if IsNil([]int{}) {
		t.Fatal("empty slice value should not be nil")
	}
}

func TestIsZero(t *testing.T) {
	// zero struct
	var v sample
	if !IsZero(v) {
		t.Fatal("zero struct should be zero")
	}
	v.A = 1
	if IsZero(v) {
		t.Fatal("non-zero struct should not be zero")
	}

	// time.Time IsZero behavior aligns with reflect.Value.IsZero
	var tm time.Time
	if !IsZero(tm) {
		t.Fatal("zero time should be zero")
	}
	tm = time.Now()
	if IsZero(tm) {
		t.Fatal("non-zero time should not be zero")
	}

	// slice: only nil is zero; empty but non-nil is not zero
	var s []int
	if !IsZero(s) {
		t.Fatal("nil slice should be zero by reflect")
	}
	s = []int{}
	if IsZero(s) {
		t.Fatal("empty (non-nil) slice should not be zero by reflect")
	}
}

func TestNilOrZero(t *testing.T) {
	var p *sample
	if !NilOrZero(p) {
		t.Fatal("nil pointer should be NilOrZero")
	}
	x := sample{}
	p = &x
	if !NilOrZero(p) {
		t.Fatal("pointer to zero value should be NilOrZero")
	}
	x.A = 2
	if NilOrZero(p) {
		t.Fatal("pointer to non-zero value should not be NilOrZero")
	}
}

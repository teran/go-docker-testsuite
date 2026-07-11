package ptr

import (
	"testing"
)

func TestPtr(t *testing.T) {
	v := 42
	p := Ptr(v)
	if *p != v {
		t.Errorf("Ptr(%d) = %d, want %d", v, *p, v)
	}
	if p == nil {
		t.Error("Ptr() returned nil")
	}
}

func TestPtrString(t *testing.T) {
	v := "hello"
	p := Ptr(v)
	if *p != v {
		t.Errorf("Ptr(%s) = %s, want %s", v, *p, v)
	}
}

func TestPtrBool(t *testing.T) {
	v := true
	p := Ptr(v)
	if *p != v {
		t.Errorf("Ptr(%v) = %v, want %v", v, *p, v)
	}
}

func TestPtrZeroValue(t *testing.T) {
	var v int64 = 0
	p := Ptr(v)
	if *p != 0 {
		t.Errorf("Ptr(0) = %d, want 0", *p)
	}
}

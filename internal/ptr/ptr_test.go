package ptr

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPtr(t *testing.T) {
	r := require.New(t)

	v := 42
	p := Ptr(v)
	r.NotNil(p)
	r.Equal(v, *p)
}

func TestPtrString(t *testing.T) {
	r := require.New(t)

	v := "hello"
	p := Ptr(v)
	r.NotNil(p)
	r.Equal(v, *p)
}

func TestPtrBool(t *testing.T) {
	r := require.New(t)

	v := true
	p := Ptr(v)
	r.NotNil(p)
	r.Equal(v, *p)
}

func TestPtrZeroValue(t *testing.T) {
	r := require.New(t)

	var v int64 = 0
	p := Ptr(v)
	r.NotNil(p)
	r.Equal(int64(0), *p)
}

//go:build unit

package docker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHostPortString(t *testing.T) {
	r := require.New(t)

	hp := HostPort{
		Host: "11.1.1.1",
		Port: 123,
	}

	r.Equal("11.1.1.1:123", hp.String())
}

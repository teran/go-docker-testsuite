package images

import (
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

func TestImageName(t *testing.T) {
	r := require.New(t)

	err := os.Unsetenv("IMAGE_PREFIX")
	r.NoError(err)

	v := ImageName("testdata")
	r.Equal("testdata", v)

	err = os.Setenv("IMAGE_PREFIX", "some-proxy.example.com")
	r.NoError(err)

	v = ImageName("testdata")
	r.Equal("some-proxy.example.com/testdata", v)
}

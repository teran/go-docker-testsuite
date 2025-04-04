package docker

import (
	"regexp"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

func TestSubstringMatcher(t *testing.T) {
	r := require.New(t)

	m := NewSubstringMatcher("blah")

	r.False(m("some_unexpected"))
	r.True(m("test blah test"))
	r.True(m("blah"))
}

func TestExactMatcher(t *testing.T) {
	r := require.New(t)

	m := NewExactMatcher("blah")

	r.False(m("test_string"))
	r.False(m("test blah test"))
	r.True(m("blah"))
}

func TestRegexpMatcher(t *testing.T) {
	r := require.New(t)

	m := NewRegexpMatcher(regexp.MustCompile("^blah$"))

	r.False(m("blah "))
	r.False(m(" blah"))
	r.True(m("blah"))
}

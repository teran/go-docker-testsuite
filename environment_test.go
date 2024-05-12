//go:build unit

package docker

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func init() {
	log.SetLevel(log.TraceLevel)
}

func TestEnvironmentBuilder(t *testing.T) {
	r := require.New(t)

	e := NewEnvironment().
		StringVar("string_var", "string_value").
		IntVar("int_var", int(1234)).
		Int64Var("int64_var", int64(5678)).
		Int32Var("int32_var", int32(9012)).
		Int16Var("int16_var", int16(3456)).
		Int8Var("int8_var", int8(126)).
		UintVar("uint_var", uint(987)).
		Uint64Var("uint64_var", uint64(654)).
		Uint32Var("uint32_var", uint32(321)).
		Uint16Var("uint16_var", uint16(9087)).
		Uint8Var("uint8_var", uint8(255)).
		BoolVar("bool_var", true)

	r.Equal(Environment{
		"string_var": "string_value",
		"int_var":    "1234",
		"int64_var":  "5678",
		"int32_var":  "9012",
		"int16_var":  "3456",
		"int8_var":   "126",
		"uint_var":   "987",
		"uint64_var": "654",
		"uint32_var": "321",
		"uint16_var": "9087",
		"uint8_var":  "255",
		"bool_var":   "true",
	}, e)

	r.ElementsMatch([]string{
		"string_var=string_value",
		"int_var=1234",
		"int64_var=5678",
		"int32_var=9012",
		"int16_var=3456",
		"int8_var=126",
		"uint_var=987",
		"uint64_var=654",
		"uint32_var=321",
		"uint16_var=9087",
		"uint8_var=255",
		"bool_var=true",
	}, e.list())
}

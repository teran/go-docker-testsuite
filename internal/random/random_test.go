package random

import (
	"testing"
	"unicode"

	"github.com/stretchr/testify/require"
)

func TestStringLength(t *testing.T) {
	for _, tc := range []struct {
		set     []rune
		len     uint
		setName string
	}{
		{Numeric, 0, "numeric0"},
		{Numeric, 1, "numeric1"},
		{Numeric, 10, "numeric10"},
		{AlphaLower, 5, "alphalower5"},
		{AlphaNumeric, 8, "alphanumeric8"},
	} {
		t.Run(tc.setName, func(t *testing.T) {
			r := require.New(t)

			s := String(tc.set, tc.len)
			r.Len(s, int(tc.len))

			for _, rn := range s {
				r.True(containsRune(tc.set, rn), "String() = %q contains %c not in set", s, rn)
			}
		})
	}
}

func TestStringValidRunes(t *testing.T) {
	r := require.New(t)

	s := String(AlphaNumeric, 100)
	r.Len(s, 100)

	for _, rn := range s {
		r.True(unicode.IsLetter(rn) || unicode.IsDigit(rn), "String(AlphaNumeric) contains %c (%U)", rn, rn)
	}
}

func containsRune(set []rune, r rune) bool {
	for _, v := range set {
		if v == r {
			return true
		}
	}
	return false
}

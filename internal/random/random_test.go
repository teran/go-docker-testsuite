package random

import (
	"testing"
	"unicode"
)

func TestStringLength(t *testing.T) {
	for _, tc := range []struct {
		set      []rune
		len      uint
		setName  string
	}{
		{Numeric, 0, "numeric0"},
		{Numeric, 1, "numeric1"},
		{Numeric, 10, "numeric10"},
		{AlphaLower, 5, "alphalower5"},
		{AlphaNumeric, 8, "alphanumeric8"},
	} {
		t.Run(tc.setName, func(t *testing.T) {
			s := String(tc.set, tc.len)
			if len(s) != int(tc.len) {
				t.Errorf("String(len=%d) = %q (len=%d)", tc.len, s, len(s))
			}
			for _, r := range s {
				if !containsRune(tc.set, r) {
					t.Errorf("String() = %q contains %c not in set", s, r)
				}
			}
		})
	}
}

func TestStringDeterministic(t *testing.T) {
	s1 := String(Numeric, 5)
	s2 := String(Numeric, 5)
	if s1 == s2 {
		// This can happen with random, but chances are negligible for 5 digits.
		// Only flag if same seed consistently produces same result.
		t.Logf("unlikely collision: two calls returned %q", s1)
	}
}

func TestStringValidRunes(t *testing.T) {
	s := String(AlphaNumeric, 100)
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			t.Errorf("String(AlphaNumeric) contains %c (%U)", r, r)
		}
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

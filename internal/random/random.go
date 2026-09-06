package random

import "math/rand/v2"

var (
	Numeric      = []rune("0123456789")
	AlphaLower   = []rune("abcdefghijklmnopqrstuvwxyz")
	AlphaNumeric = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
)

func String(set []rune, l uint) string {
	s := make([]rune, l)
	for i := range s {
		// #nosec G404 -- random suffixes are used as unique container/group
		// identifiers, not as security-critical tokens; crypto/rand would be
		// needless overhead here.
		s[i] = set[rand.IntN(len(set))]
	}
	return string(s)
}

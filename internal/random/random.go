package random

import (
	"math/rand"
	"time"
)

var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

var (
	Numeric      = []rune("0123456789")
	AlphaLower   = []rune("abcdefghijklmnopqrstuvwxyz")
	AlphaNumeric = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
)

func String(set []rune, l uint) string {
	s := make([]rune, l)
	for i := range s {
		s[i] = set[rng.Intn(len(set))]
	}
	return string(s)
}

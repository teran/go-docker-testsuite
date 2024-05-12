package docker

import (
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
)

// Matcher allows to create any kind of matcher for container outputs
type Matcher func(l string) bool

// NewSubstringMatcher represents partial matcher
func NewSubstringMatcher(s string) Matcher {
	return func(l string) bool {
		ok := strings.Contains(l, s)

		log.WithFields(log.Fields{
			"kind":    "substring",
			"pattern": s,
			"line":    l,
			"result":  ok,
		}).Trace("matching string")

		return ok
	}
}

// NewExactMatcher represents exact matcher i.e. the output should be
// exactly matched (except space chars around the word)
func NewExactMatcher(s string) Matcher {
	return func(l string) bool {
		ok := strings.TrimSpace(l) == s

		log.WithFields(log.Fields{
			"kind":    "exact",
			"pattern": s,
			"line":    l,
			"result":  ok,
		}).Trace("matching string")

		return ok
	}
}

func NewRegexpMatcher(r *regexp.Regexp) Matcher {
	return func(l string) bool {
		return r.MatchString(l)
	}
}

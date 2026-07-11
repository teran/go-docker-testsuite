// split_test_groups discovers Go test packages and splits them into
// balanced CI matrix groups.
//
// Outputs a JSON object suitable for GitHub Actions fromJson:
//
//	{"include": [
//	  {"name":"core","check_coverage":true, "timeout_minutes":10, "packages":". ./internal/..."},
//	  {"name":"g00", "check_coverage":false,"timeout_minutes":25, "packages":"pkg/a pkg/b ..."}
//	]}
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

const module = "github.com/teran/go-docker-testsuite"

type group struct {
	Name           string `json:"name"`
	Packages       string `json:"packages"`
	CheckCoverage  bool   `json:"check_coverage"`
	TimeoutMinutes int    `json:"timeout_minutes"`
}

type matrix struct {
	Include []group `json:"include"`
}

func main() {
	nGroups := 5
	if len(os.Args) > 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s\nNo arguments accepted; groups count is fixed at %d.\n", os.Args[0], nGroups)
		os.Exit(1)
	}

	corePkgs, otherPkgs := discover()

	include := []group{
		{
			Name:           "core",
			Packages:       strings.Join(corePkgs, " "),
			CheckCoverage:  true,
			TimeoutMinutes: 10,
		},
	}

	buckets := make([][]string, nGroups)
	for i, pkg := range otherPkgs {
		bucket := i % nGroups
		buckets[bucket] = append(buckets[bucket], pkg)
	}

	for i := 0; i < nGroups; i++ {
		if len(buckets[i]) == 0 {
			continue
		}
		sort.Strings(buckets[i])

		include = append(include, group{
			Name:           fmt.Sprintf("g%02d", i),
			Packages:       strings.Join(buckets[i], " "),
			CheckCoverage:  false,
			TimeoutMinutes: 25,
		})
	}

	if err := json.NewEncoder(os.Stdout).Encode(matrix{Include: include}); err != nil {
		fmt.Fprintf(os.Stderr, "error encoding JSON: %v\n", err)
		os.Exit(1)
	}
}

// discover runs "go list" to find test packages and splits them into
// core (root + internal/...) and others.
func discover() (core, other []string) {
	cmd := exec.Command("go", "list", "-f", "{{if .TestGoFiles}}{{.ImportPath}}{{end}}", "./...")
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error running go list: %v\n", err)
		os.Exit(1)
	}

	for _, p := range strings.Fields(string(out)) {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		// Root package or internal/ → core
		if p == module || strings.HasPrefix(p, module+"/internal/") {
			rel := p
			if p == module {
				rel = "."
			} else {
				rel = strings.TrimPrefix(p, module+"/")
			}
			core = append(core, rel)
			continue
		}

		// Everything else → other (strip module prefix for go test)
		rel := strings.TrimPrefix(p, module+"/")
		other = append(other, rel)
	}

	sort.Strings(core)
	sort.Strings(other)
	return core, other
}

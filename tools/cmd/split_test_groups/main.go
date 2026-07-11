// split_test_groups discovers Go test packages and groups them into
// balanced CI matrix groups by application type.
//
// Outputs a JSON object suitable for GitHub Actions fromJson:
//
//	{"include": [
//	  {"name":"core",       "check_coverage":true,  "timeout_minutes":10, "packages":". ./internal/..."},
//	  {"name":"postgres",   "check_coverage":false, "timeout_minutes":25, "packages":"./applications/postgres ./applications/postgres/versions/..."},
//	  {"name":"datastores", "check_coverage":false, "timeout_minutes":25, "packages":"./applications/mysql/versions/... ./applications/redis/versions/..."},
//	  ...
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
	if len(os.Args) > 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s\nNo arguments accepted.\n", os.Args[0])
		os.Exit(1)
	}

	allPkgs := discover()

	// Categorise every discovered (import-path → relative-dir).
	type entry struct {
		importPath string // e.g. github.com/teran/.../applications/postgres
		relDir     string // e.g. ./applications/postgres
		app        string // top-level directory under applications/ – e.g. postgres
	}
	var entries []entry
	for _, p := range allPkgs {
		rel := relPath(p) // "./applications/postgres" or just "."
		app := appName(rel)
		entries = append(entries, entry{importPath: p, relDir: rel, app: app})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].relDir < entries[j].relDir
	})

	// ── Build group buckets ────────────────────────────────────────
	type bucket struct {
		name string
		dirs []string
	}

	// 1. core: root + internal
	core := &bucket{name: "core", dirs: []string{".", "./internal/..."}}
	var rest []entry
	for _, e := range entries {
		if e.relDir == "." || strings.HasPrefix(e.relDir, "./internal/") {
			continue
		}
		rest = append(rest, e)
	}

	// 2. Group by application name (map preserves insertion order)
	appBuckets := make([]*bucket, 0, 8)
	appIndex := map[string]*bucket{}
	for _, e := range rest {
		b, ok := appIndex[e.app]
		if !ok {
			b = &bucket{name: e.app}
			appBuckets = append(appBuckets, b)
			appIndex[e.app] = b
		}
		b.dirs = append(b.dirs, e.relDir)
	}

	// Smash individual dirs into a single `./applications/<app>/versions/...`
	// glob when they're all version sub‑packages. Otherwise keep them
	// as explicit dirs but collapse contiguous runs into `...` wildcards.
	for _, b := range appBuckets {
		b.dirs = collapse(b.dirs)
	}

	include := []group{
		{
			Name:           core.name,
			Packages:       strings.Join(core.dirs, " "),
			CheckCoverage:  true,
			TimeoutMinutes: 10,
		},
	}
	for _, b := range appBuckets {
		include = append(include, group{
			Name:           b.name,
			Packages:       strings.Join(b.dirs, " "),
			CheckCoverage:  false,
			TimeoutMinutes: 25,
		})
	}

	if err := json.NewEncoder(os.Stdout).Encode(matrix{Include: include}); err != nil {
		fmt.Fprintf(os.Stderr, "error encoding JSON: %v\n", err)
		os.Exit(1)
	}
}

// discover runs "go list" and returns every import path that has test files
// (internal or external test packages).
func discover() []string {
	cmd := exec.Command("go", "list", "-f", "{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}", "./...")
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error running go list: %v\n", err)
		os.Exit(1)
	}

	var pkgs []string
	for _, p := range strings.Fields(string(out)) {
		p = strings.TrimSpace(p)
		if p != "" {
			pkgs = append(pkgs, p)
		}
	}
	sort.Strings(pkgs)
	return pkgs
}

// relPath converts a full import path to a relative directory path
// suitable for "go test". e.g.:
//
//	"github.com/teran/go-docker-testsuite"                      → "."
//	"github.com/teran/.../internal/ptr"                          → "./internal/ptr"
//	"github.com/teran/.../applications/postgres"                 → "./applications/postgres"
//	"github.com/teran/.../applications/postgres/versions/17.4"  → "./applications/postgres/versions/17.4"
func relPath(pkg string) string {
	if pkg == module {
		return "."
	}
	return "./" + strings.TrimPrefix(pkg, module+"/")
}

// appName extracts the top‑level application directory name from a
// relative path. For paths outside applications/ it returns "misc".
//
//	"./applications/postgres"                 → "postgres"
//	"./applications/postgres/versions/17.4"   → "postgres"
//	"./internal/ptr"                          → "misc"
func appName(rel string) string {
	rel = strings.TrimPrefix(rel, "./")
	parts := strings.SplitN(rel, "/", 3)
	if len(parts) >= 2 && parts[0] == "applications" {
		return parts[1]
	}
	return "misc"
}

// collapse collapses a sorted list of dirs into the shortest set of
// globs. When every entry lives under a single parent with a `versions/`
// subdirectory the whole lot is replaced by a single <parent>/versions/...
// glob. Otherwise individual dirs are kept as-is.
//
// Input is already sorted.
func collapse(dirs []string) []string {
	if len(dirs) <= 2 {
		return dirs
	}

	// Check whether all dirs share a common parent that has a versions/
	// subdirectory.
	parent := commonParent(dirs)
	if parent != "" {
		// If there's a dir that IS the parent itself (e.g. ./applications/postgres),
		// include it too.
		var result []string
		hasParent := false
		for _, d := range dirs {
			if d == parent {
				hasParent = true
				break
			}
		}
		if hasParent {
			result = append(result, parent)
		}
		result = append(result, parent+"/versions/...")
		return result
	}

	return dirs
}

// commonParent returns the common "<parent>/versions/..." parent if all
// version sub‑packages share the same applications/<app>/ prefix.
// The parent dir itself (e.g. "./applications/postgres") is ignored when
// present — it only checks the version sub‑packages.
func commonParent(dirs []string) string {
	var subDirs []string
	for _, d := range dirs {
		parts := strings.Split(strings.TrimPrefix(d, "./"), "/")
		// Keep only entries that look like version sub‑packages
		// (applications/<app>/<something>/...)
		if len(parts) >= 3 {
			subDirs = append(subDirs, d)
		}
	}
	if len(subDirs) < 2 {
		return ""
	}

	parts0 := strings.Split(strings.TrimPrefix(subDirs[0], "./"), "/")
	app := parts0[0] + "/" + parts0[1] // e.g. "applications/postgres"

	for _, d := range subDirs[1:] {
		parts := strings.Split(strings.TrimPrefix(d, "./"), "/")
		if parts[0]+"/"+parts[1] != app {
			return ""
		}
	}

	return "./" + app
}



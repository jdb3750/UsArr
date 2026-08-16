//go:build bench

package db

import "strings"

// This file exists ONLY under the `bench` build tag, so nothing it exports can
// reach a shipped binary. It is the seam the RSS measurement harness
// (internal/db/spike, ARCHITECTURE §13, ADR-0001) needs: the harness must
// measure the pragmas Open actually sets, not a re-implementation of them, and
// two of those pragmas — cache_size and mmap_size — are the values it exists to
// decide. Copying the DSN construction into the harness would measure the copy.

// SetPragmaForSpike overrides the value of a pragma already present in the
// package's pragma list, before Open builds its DSN. It reports whether the
// pragma was found; an unknown name is a caller bug, not a silent no-op.
//
// Not safe for concurrent use, and not meant to be: the spike calls it once at
// process start, then opens exactly one database.
func SetPragmaForSpike(name, value string) bool {
	prefix := name + "("
	for i, p := range pragmas {
		if strings.HasPrefix(p, prefix) {
			pragmas[i] = prefix + value + ")"
			return true
		}
	}
	return false
}

// PragmasForSpike returns a copy of the pragma list Open will apply, so the
// spike's report states what was actually measured rather than what the reader
// assumes.
func PragmasForSpike() []string {
	return append([]string(nil), pragmas...)
}

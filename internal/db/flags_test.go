package db

import "flag"

// flagBool registers a test-only flag. It lives here rather than inline so the
// -update-schema flag is registered exactly once regardless of test ordering.
func flagBool(name, usage string) *bool { return flag.Bool(name, false, usage) }

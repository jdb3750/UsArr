package config

import "os"

// readRepoFile reads a file relative to this package's directory. Tests use it
// to assert against the shipped .env.example.
func readRepoFile(path string) ([]byte, error) { return os.ReadFile(path) }

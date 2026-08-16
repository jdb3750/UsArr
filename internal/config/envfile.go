package config

import (
	"fmt"
	"strings"
)

// ParseEnvFile parses an env file into a map.
//
// The file is DATA, not shell. It is deliberately never sourced with
// `. ./.env`: bash performs parameter expansion and command substitution and
// Docker Compose's env_file parser does neither, so the same file would mean
// two different things — on a file that can hold the master key. This parser is
// the only one, and `make dev` calls the binary with --env-file rather than
// sourcing.
//
// The grammar is deliberately small:
//
//   - Blank lines, and lines whose first non-space character is '#', are ignored.
//   - Every other line is KEY=VALUE. A line with no '=' is an error, not a
//     silently skipped line.
//   - KEY matches [A-Za-z_][A-Za-z0-9_]*.
//   - A value wrapped in single quotes is taken literally.
//   - A value wrapped in double quotes has \n, \r, \t, \\ and \" unescaped.
//   - An unquoted value is taken literally after trailing whitespace is trimmed.
//
// There is no variable expansion, no command substitution, no `export` prefix
// and no inline-comment stripping. Inline comments are omitted on purpose: a
// base64 master key cannot contain '#', but a stored password can, and a parser
// that truncates a value at a '#' would corrupt it silently. Put comments on
// their own line.
func ParseEnvFile(content string) (map[string]string, error) {
	out := map[string]string{}
	for i, line := range strings.Split(content, "\n") {
		lineNo := i + 1
		trimmed := strings.TrimLeft(strings.TrimSuffix(line, "\r"), " \t")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: %q is not KEY=VALUE", lineNo, trimmed)
		}
		key = strings.TrimRight(key, " \t")
		if !validKey(key) {
			return nil, fmt.Errorf("line %d: %q is not a valid variable name", lineNo, key)
		}

		value, err := unquote(value)
		if err != nil {
			return nil, fmt.Errorf("line %d: %s: %w", lineNo, key, err)
		}
		out[key] = value
	}
	return out, nil
}

func validKey(k string) bool {
	if k == "" {
		return false
	}
	for i, r := range k {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

func unquote(raw string) (string, error) {
	v := strings.TrimLeft(raw, " \t")
	if len(v) >= 2 {
		switch q := v[0]; q {
		case '\'':
			if v[len(v)-1] != '\'' {
				return "", fmt.Errorf("unterminated single quote")
			}
			return v[1 : len(v)-1], nil
		case '"':
			if v[len(v)-1] != '"' {
				return "", fmt.Errorf("unterminated double quote")
			}
			return unescape(v[1 : len(v)-1]), nil
		}
	}
	return strings.TrimRight(v, " \t"), nil
}

func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '\\', '"':
			b.WriteByte(s[i])
		default:
			// Unknown escape: keep both characters rather than eat the
			// backslash, so a Windows path in a quoted value survives.
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

package config

import (
	"strings"
	"testing"
)

func TestParseEnvFile(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want map[string]string
	}{
		{
			name: "basic pairs",
			in:   "USARR_PORT=8484\nUSARR_LOG_LEVEL=info\n",
			want: map[string]string{"USARR_PORT": "8484", "USARR_LOG_LEVEL": "info"},
		},
		{
			name: "blank lines and comments are ignored",
			in:   "\n# a comment\n   # indented comment\nA=1\n\n",
			want: map[string]string{"A": "1"},
		},
		{
			name: "empty value is allowed",
			in:   "USARR_URL_BASE=\n",
			want: map[string]string{"USARR_URL_BASE": ""},
		},
		{
			name: "leading whitespace on the key is trimmed",
			in:   "   A=1\nB   =2\n",
			want: map[string]string{"A": "1", "B": "2"},
		},
		{
			name: "trailing whitespace on an unquoted value is trimmed",
			in:   "A=1   \n",
			want: map[string]string{"A": "1"},
		},
		{
			name: "single quotes are literal",
			in:   `A='hello $USER \n world'` + "\n",
			want: map[string]string{"A": `hello $USER \n world`},
		},
		{
			name: "double quotes unescape",
			in:   `A="line1\nline2\ttab"` + "\n",
			want: map[string]string{"A": "line1\nline2\ttab"},
		},
		{
			name: "quotes preserve whitespace",
			in:   `A="  padded  "` + "\n",
			want: map[string]string{"A": "  padded  "},
		},
		{
			// No expansion, ever. The file is data, not shell: bash would
			// expand these and Docker Compose would not, so the same file would
			// mean two different things.
			name: "no parameter expansion",
			in:   "A=$HOME\nB=${HOME}\n",
			want: map[string]string{"A": "$HOME", "B": "${HOME}"},
		},
		{
			// No command substitution either.
			name: "no command substitution",
			in:   "A=$(rm -rf /)\nB=`whoami`\n",
			want: map[string]string{"A": "$(rm -rf /)", "B": "`whoami`"},
		},
		{
			// A '#' inside a value survives. A base64 key cannot contain one,
			// but a stored password can, and truncating at '#' would corrupt it
			// silently.
			name: "hash inside a value is not an inline comment",
			in:   "A=pa#ssword\nB=trailing # not a comment\n",
			want: map[string]string{"A": "pa#ssword", "B": "trailing # not a comment"},
		},
		{
			name: "value containing = keeps everything after the first",
			in:   "A=b=c=d\n",
			want: map[string]string{"A": "b=c=d"},
		},
		{
			name: "base64 padding survives",
			in:   "USARR_SECRET_KEY=" + fixtureKey + "\n",
			want: map[string]string{"USARR_SECRET_KEY": fixtureKey},
		},
		{
			name: "CRLF line endings",
			in:   "A=1\r\nB=2\r\n",
			want: map[string]string{"A": "1", "B": "2"},
		},
		{
			name: "no trailing newline",
			in:   "A=1",
			want: map[string]string{"A": "1"},
		},
		{
			// An escape the parser does not know keeps both characters rather
			// than eating the backslash.
			name: "unknown escape keeps the backslash",
			in:   `A="C:\Program\Files"` + "\n",
			want: map[string]string{"A": `C:\Program\Files`},
		},
		{
			// But \t inside double quotes IS a tab, so a Windows path with a
			// \t in it must use single quotes. Pinned so nobody "fixes" the
			// unescaper into corrupting one silently.
			name: "double-quoted backslash-t is a tab, single quotes are literal",
			in:   `A="C:\temp"` + "\n" + `B='C:\temp'` + "\n",
			want: map[string]string{"A": "C:\temp", "B": `C:\temp`},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseEnvFile(tc.in)
			if err != nil {
				t.Fatalf("ParseEnvFile: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d keys %v, want %d %v", len(got), got, len(tc.want), tc.want)
			}
			for k, want := range tc.want {
				if got[k] != want {
					t.Errorf("%s = %q, want %q", k, got[k], want)
				}
			}
		})
	}
}

func TestParseEnvFileErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// A line with no '=' is an error rather than a skipped line: silently
		// ignoring it is how a mistyped master key line goes unnoticed.
		{name: "no equals", in: "USARR_PORT 8484\n", want: "not KEY=VALUE"},
		{name: "export prefix unsupported", in: "export A=1\n", want: "not a valid variable name"},
		{name: "empty key", in: "=1\n", want: "not a valid variable name"},
		{name: "key starting with a digit", in: "1A=1\n", want: "not a valid variable name"},
		{name: "key with a dash", in: "A-B=1\n", want: "not a valid variable name"},
		{name: "unterminated double quote", in: `A="oops` + "\n", want: "unterminated"},
		{name: "unterminated single quote", in: "A='oops\n", want: "unterminated"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseEnvFile(tc.in)
			if err == nil {
				t.Fatalf("ParseEnvFile = nil error, want %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

// The shipped .env.example must parse with this parser: it is the file every
// operator copies, and a parser that chokes on it is a broken first run.
func TestParseShippedEnvExample(t *testing.T) {
	raw, err := readRepoFile("../../.env.example")
	if err != nil {
		t.Skipf("no .env.example: %v", err)
	}
	env, err := ParseEnvFile(string(raw))
	if err != nil {
		t.Fatalf("ParseEnvFile(.env.example): %v", err)
	}
	// Every uncommented line is a real variable, and no key line is uncommented.
	if _, ok := env["USARR_SECRET_KEY"]; ok {
		t.Error(".env.example has an uncommented USARR_SECRET_KEY line; it must never ship one")
	}
	for _, want := range []string{"USARR_CONFIG_DIR", "USARR_PORT", "USARR_LOG_LEVEL", "TZ"} {
		if _, ok := env[want]; !ok {
			t.Errorf(".env.example does not set %s", want)
		}
	}
}

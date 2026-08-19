package vcrscrub

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jdb3750/UsArr/internal/ssrf"
)

// ─── The on-disk backstop ────────────────────────────────────────────────────
//
// Scrub stops a credential being WRITTEN. This stops one being COMMITTED, and it
// is a separate mechanism on purpose: the hook only protects a recording that
// went through New, and the whole failure mode being defended against is someone
// wiring a recorder by hand — which is what every caller in this tree did until
// New existed.
//
// `make secrets` is not this, and the difference was measured rather than
// assumed. gitleaks under this repo's .gitleaks.toml, one freshly generated
// uuid4 per probe, clean baseline in between:
//
//	url: …/api/Image/series-cover?seriesId=1&apiKey=<guid>   → exit 1, "leaks found: 1"
//	url: …/api/Opds/<the same guid>/series                   → exit 0, "no leaks found"
//
// It fires on the `apiKey=` keyword sitting next to the value, not on the value.
// So the labelled half of the class is already covered by the gate and the
// UNLABELLED half is not — which is precisely the half Kavita's OPDS routes
// produce.
//
// TIER 1 is name-based and has no false positives: it asks
// ssrf.IsCredentialParam, the ONE deny-list, whether a `name=value` or
// `"name": "value"` pair names a credential, and then demands the value be a
// placeholder. A parameter literally called `apiKey` has no business holding a
// live value in a fixture, whatever the value looks like.
//
// TIER 2 is shape-based and exists for the credential with NO name attached:
// Kavita's OPDS routes carry the Auth Key as a bare path segment, where tier 1
// has nothing to match on. It flags a canonical GUID or a run of 32-or-more hex
// characters anywhere in the file.

// hexBlobFields are JSON field names whose value is legitimately a long hex run
// or a GUID, and which tier 2 therefore does not flag.
//
// It is short, and it is meant to stay short. Its polarity is the point: a new
// fixture field carrying a hex blob makes the suite RED and someone has to look
// at it and decide. That is the opposite of a value allowlist, which would have
// to be edited for every new fixture and would quietly grow to cover a real key.
var hexBlobFields = map[string]struct{}{
	// A BitTorrent infohash: SHA-1 of the torrent's info dictionary, 40 hex. It
	// is public by construction — it is what you hand a DHT — and Prowlarr's
	// ReleaseResource carries it on every result.
	"infohash": {},
	// Kavita's install identifier, a GUID. Not a credential: ServerInfoDto
	// reports it and nothing authenticates with it.
	"installid": {},
}

var (
	guidAnywhereRe = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	longHexRe      = regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`)
)

// Finding is one suspected credential in one cassette.
type Finding struct {
	File   string
	Line   int
	Tier   int // 1 = named credential parameter, 2 = credential-shaped value
	Detail string
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: [tier %d] %s", f.File, f.Line, f.Tier, f.Detail)
}

// ScanDir walks dir for *.yaml cassettes and reports every suspected credential.
//
// It reads the RAW TEXT rather than go-vcr's parsed structs. A parser only sees
// the fields it models, and a leak does not agree to sit in one of them — a
// credential in a YAML comment, in an unmodelled key or in a trailer is still a
// credential in the repository.
func ScanDir(dir string) ([]Finding, error) {
	var out []Finding
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}
		b, err := os.ReadFile(path) //nolint:gosec // fixture paths, test-only
		if err != nil {
			return err
		}
		out = append(out, ScanText(filepath.Base(path), string(b))...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ScanText applies both tiers to one cassette's text.
func ScanText(name, text string) []Finding {
	var out []Finding
	for n, line := range strings.Split(text, "\n") {
		lineNo := n + 1

		// Tier 1: a value sitting under a credential parameter name.
		for _, g := range queryPairRe.FindAllStringSubmatch(line, -1) {
			if ssrf.IsCredentialParam(g[1]) && !isPlaceholder(g[2]) {
				out = append(out, Finding{name, lineNo, 1, fmt.Sprintf(
					"query parameter %q carries %q, not a placeholder", g[1], g[2])})
			}
		}
		exempt := map[string]struct{}{}
		for _, g := range jsonPairRe.FindAllStringSubmatch(line, -1) {
			if _, ok := hexBlobFields[strings.ToLower(g[1])]; ok {
				exempt[g[3]] = struct{}{}
				continue
			}
			if ssrf.IsCredentialParam(g[1]) && !isPlaceholder(g[3]) {
				out = append(out, Finding{name, lineNo, 1, fmt.Sprintf(
					"JSON field %q carries %q, not a placeholder", g[1], g[3])})
			}
		}

		// Tier 2: a credential-SHAPED value with no name to judge it by.
		for _, m := range append(guidAnywhereRe.FindAllString(line, -1), longHexRe.FindAllString(line, -1)...) {
			if _, ok := exempt[m]; ok {
				continue
			}
			out = append(out, Finding{name, lineNo, 2, fmt.Sprintf(
				"credential-shaped value %q — a Kavita Auth Key is a bare GUID and an *Arr key is 32 hex; "+
					"if this is genuinely not a secret, add its field name to hexBlobFields with the reason", m)})
		}
	}
	return out
}

func isPlaceholder(v string) bool {
	return slices.Contains(PlaceholderValues, strings.TrimSpace(v))
}

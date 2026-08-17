package kavita

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Timestamp is a .NET DateTime as Kavita puts it on the wire, and it exists
// because Go's time.Time CANNOT PARSE HALF OF THEM.
//
// # The failure, found by firing it rather than by reading the spec
//
// The vendored OpenAPI document declares every one of these `format: date-time`,
// which reads as RFC 3339 and is what encoding/json requires — it unmarshals a
// time.Time with time.RFC3339 and nothing else. But System.Text.Json serialises a
// .NET DateTime according to its Kind, and a DateTime whose Kind is Unspecified —
// which is what EF Core hands back for an ordinary DATETIME column — is written
// with NO ZONE AT ALL:
//
//	"lastChapterAdded":    "2026-08-17T03:00:30.118"     ← no offset, no Z
//	"lastChapterAddedUtc": "2026-08-17T07:00:30.118Z"    ← or possibly also none
//
// Unmarshalling the first into a time.Time fails with
// `cannot parse "" as "Z07:00"`, and because encoding/json aborts the whole
// document on it, ONE such field makes the ENTIRE series list undecodable. This
// is docs/DEVELOPMENT.md §5's rule in a new place: the spec says date-time, the
// serialiser says otherwise, and the serialiser wins.
//
// ⚠️ WHAT IS AND IS NOT VERIFIED. That the offset-less form occurs is certain —
// it is what `latestReadDate`'s .NET default `0001-01-01T00:00:00` looks like on
// any instance. WHICH fields carry a zone on a real Kavita is NOT verified from a
// live capture: this project's one live Kavita run (ADR-0035 §2a) recorded the
// ordering and the ids, not the raw timestamp bytes. So every date-time field is
// this type, and the type accepts both forms rather than guessing per field.
//
// # Why HasZone is exported rather than smoothed away
//
// An offset-less timestamp is a WALL CLOCK on the Kavita host. It is not an
// instant, and pretending it is one is how a replica ends up comparing a value
// from a UTC+2 server against a UTC watermark and deciding a series changed six
// hours ago. Time() returns something usable, HasZone() says whether it means
// anything across machines, and the caller has to look. Channel 3b's watermark
// must use LastChapterAddedUtc for exactly this reason.
type Timestamp struct {
	t       time.Time
	hasZone bool
	raw     string
}

// zonelessLayouts are the offset-less forms System.Text.Json produces for a
// DateTime with Kind=Unspecified. .NET trims trailing zeros from the fractional
// part, so the precision varies and time.Parse's ".999999999" handles the range.
var zonelessLayouts = []string{
	"2006-01-02T15:04:05.999999999",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

// UnmarshalJSON accepts RFC 3339, the offset-less .NET forms, JSON null and the
// empty string.
//
// It NEVER fails on a value it does not recognise. That is a deliberate
// asymmetry: this package's other bounds fail rather than degrade, because a
// truncated list is indistinguishable from a deletion — but a timestamp it cannot
// parse is one unusable field, and failing the decode would throw away the whole
// series list over it. The raw bytes are kept so the caller can see what arrived.
func (ts *Timestamp) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*ts = Timestamp{}
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("%w: a date-time property arrived as %s, which is not a JSON string",
			ErrDecode, truncate(string(b)))
	}
	*ts = ParseTimestamp(s)
	return nil
}

// ParseTimestamp turns one wire value into a Timestamp. Exported so a caller
// reading a raw JSON payload out of a support bundle can use the same rules.
func ParseTimestamp(s string) Timestamp {
	s = strings.TrimSpace(s)
	if s == "" {
		return Timestamp{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return Timestamp{t: t, hasZone: true, raw: s}
	}
	for _, layout := range zonelessLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			// Parsed in UTC because time.Parse has nowhere else to put it. That
			// label is a placeholder, NOT a claim — HasZone() is what says so.
			return Timestamp{t: t, hasZone: false, raw: s}
		}
	}
	// Unrecognised: keep the bytes, report no time. Nothing downstream should
	// silently substitute a zero instant for a value it could not read.
	return Timestamp{raw: s}
}

// MarshalJSON round-trips the value it was given.
//
// The RAW wire string is re-emitted rather than a re-rendered one, so a cassette
// or a support bundle written back out is byte-identical to what arrived. A
// re-render would quietly add a `Z` to a zoneless value and manufacture the exact
// claim this type exists to avoid making.
func (ts Timestamp) MarshalJSON() ([]byte, error) {
	if ts.raw == "" {
		return []byte("null"), nil
	}
	return json.Marshal(ts.raw)
}

// Time returns the parsed value. For a zoneless wire value it is the Kavita
// host's WALL CLOCK carrying a UTC label — check HasZone before comparing it
// against anything from another machine.
func (ts Timestamp) Time() time.Time { return ts.t }

// HasZone reports whether the wire value carried a zone offset. False means the
// instant is unknown and only the wall clock is.
func (ts Timestamp) HasZone() bool { return ts.hasZone }

// Raw returns the wire value verbatim, or "" if the property was absent or null.
func (ts Timestamp) Raw() string { return ts.raw }

// IsZero reports "no usable time here". It covers three cases that are all the
// same thing to a caller: the property was absent, it was null, and it was .NET's
// default DateTime — `0001-01-01T00:00:00`, which is what `latestReadDate` is on
// every series nobody has opened.
func (ts Timestamp) IsZero() bool { return ts.t.IsZero() || ts.t.Year() <= 1 }

// String is the raw wire value, which is what a log line should carry.
func (ts Timestamp) String() string { return ts.raw }

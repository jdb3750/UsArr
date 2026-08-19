package store

import "testing"

// TestValidImageFormat pins the vocabulary image_asset.format may hold. It is
// the unit half of the pair whose lint half is
// TestImageWritesValidateTheFormatVocabulary.
func TestValidImageFormat(t *testing.T) {
	if !ValidImageFormat(ImageFormatJPEG) {
		t.Errorf("ValidImageFormat rejected %q, the base output format (ADR-0050)", ImageFormatJPEG)
	}

	// The empty string is NOT a member. image_asset.format says "no encoded
	// bytes yet" with SQL NULL; a writer holding "" has a bug, and letting ""
	// through here would let that bug reach the column as a value that reads
	// like a format and is not one.
	if ValidImageFormat("") {
		t.Error(`ValidImageFormat accepted ""; the absent value is NULL, not an empty token`)
	}

	// Media types and file extensions are not what this column stores.
	for _, s := range []string{"image/jpeg", "jpg", "JPEG", "Jpeg", " jpeg"} {
		if ValidImageFormat(s) {
			t.Errorf("ValidImageFormat accepted %q; the column holds lowercase codec tokens "+
				"only — see migration 00008's header for why it is not a media type", s)
		}
	}

	// avif is DEFERRED, not present. If this starts failing, someone added the
	// member — which is fine and is exactly the cheap change the absent CHECK
	// constraint buys — but ADR-0050's reopening condition asks for a measured
	// binary-size delta and encode cost first, so the ADR is owed an amendment.
	if ValidImageFormat("avif") {
		t.Error(`ValidImageFormat accepts "avif": ADR-0050 defers AVIF and names a ` +
			`reopening condition. Adding the member is cheap by design, but the ADR must ` +
			`be amended rather than silently overtaken.`)
	}
}

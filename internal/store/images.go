package store

// image_asset.format's vocabulary lives here rather than in a CHECK constraint,
// and this file is the whole of it.
//
// WHY NOT A CHECK. Migration 00008's header carries the full argument; the
// short form is that the vocabulary is expected to grow — ADR-0050 defers AVIF
// with a named reopening condition rather than rejecting it — and SQLite cannot
// ALTER an existing CHECK, so a constraint written today is a 12-step rebuild
// of a table that two `work` columns reference. ADR-0039 made the same call for
// write_queue.state, and image_asset's own `role` and `state` columns already
// carry no CHECK.
//
// WHY THIS FILE SHIPPED BEFORE THERE WAS A WRITER. ADR-0039 promised the Go
// validation and it was never written, so write_queue.state is enforced nowhere
// that runs; that is the failure this file refuses to repeat. The declaration
// and the check shipped WITH the column, and
// TestImageWritesValidateTheFormatVocabulary fails the moment a writer lands
// without calling ValidImageFormat.
//
// THE WRITER HAS NOW LANDED — imagewrite.go's PosterAsset.validate — so that
// guard is no longer vacuous, and this file is no longer a promise held open.

// Image format tokens — the values image_asset.format may hold.
//
// These are CODEC TOKENS, not media types and not file extensions. 'image/jpeg'
// is what the /img surface puts in a Content-Type header, derived from this;
// 'jpg' is a file suffix. Neither is stored. See migration 00008's header for
// why the column is a token.
const (
	// ImageFormatJPEG is the base output format for the image pipeline
	// (ADR-0050), chosen because image/jpeg is in the standard library and so
	// costs the static binary no dependency at all.
	ImageFormatJPEG = "jpeg"
)

// imageFormats is every legal value of image_asset.format.
//
// Adding a member here is the ENTIRE schema-side cost of a new output codec,
// which is the property the absent CHECK constraint buys.
var imageFormats = map[string]bool{
	ImageFormatJPEG: true,
}

// ValidImageFormat reports whether s is a legal image_asset.format value.
//
// It is deliberately strict about the empty string: "" is not a member. The
// column's way of saying "this row has no encoded bytes yet" is SQL NULL, not
// an empty token, so a writer holding "" has a bug rather than a pending row.
// A caller with a genuinely unset format must write NULL.
func ValidImageFormat(s string) bool {
	return imageFormats[s]
}

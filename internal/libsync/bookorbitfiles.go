package libsync

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/jdb3750/UsArr/internal/bookorbit"
	"github.com/jdb3750/UsArr/internal/store"
)

// The BookOrbit file pass: how a BookCard's `files[]` becomes media_file rows
// and the one primary `edition` that decides whether a book is an ebook or an
// audiobook.
//
// # THE BUG THIS FIXES, stated plainly
//
// Before this pass existed, EVERY BOOKORBIT BOOK RENDERED AS AN EBOOK,
// audiobooks included, and /library/audiobooks returned none of them. Nothing
// in the adapter was wrong about audiobooks — mapBook read the m4b, MediaKind
// classified it, and remote_subtype stored the token verbatim. The gap was one
// table: store.mediaTypeOf splits Ebooks from Audiobooks on
// `MIN(edition.format = 'audiobook')` and browseAudiobookPredicate seeks
// `edition.format = 'audiobook'`, and BookOrbitSource implemented no FileSource,
// so it wrote no edition rows — and mediaTypeOf's documented answer for a book
// with no editions is Ebooks. A correct classification that reaches no row
// renders as a wrong one.
//
// # It costs no HTTP either, and the token was kept for exactly this
//
// files.go's Kavita walk is ONE GET per series, measured at ~4 ms. This one is
// zero: `files[]` is on the card the item walk already decoded
// (packages/types/src/book.ts:134), and the primary file's format token was
// stored verbatim on service_item_link.remote_subtype by the item pass
// precisely so that the edition's format needs no second read. FileRequest
// carries that token forward — its doc comment already said "THE EDITION COSTS
// ZERO EXTRA HTTP ONLY IF IT DOES" — and this pass is the one that spends it.
//
// So there is no per-item failure mode: StreamFiles returns an empty
// []FileWalkFailure always, and that is a true statement about a pass that
// cannot fail to read, not an unimplemented one.
//
// # What it writes, and the three things it deliberately does not
//
// media_file gets work_id, service_instance_id, path (the SURROGATE — see
// internal/store/files.go's header), remote_file_id and size_bytes.
//
//  1. date_added is NOT written, and its absence is a fact about the card
//     rather than a shortcut. BookFileRef has no timestamp at all — `bookFiles`
//     stores createdAt and mtime (db/schema/books.ts:64, :70) and
//     BookDetailFile exposes createdAt (packages/types/src/book.ts:166), but the
//     CARD carries neither. store.MediaFileRow stores the zero time as NULL,
//     which is the honest answer. The book's own `addedAt` is in hand and is
//     NOT substituted: when the book was added is not when the file was.
//  2. edition.narrators, duration_seconds and abridged — ADR-0031's audiobook
//     production properties. store.FileSet has no field for any of them, so
//     writing them is a change to the shared write path rather than to this
//     adapter. The narrators at least are not lost meanwhile: they land as
//     work_credit rows (bookorbitcredits.go).
//  3. work_comic_issue, and the reason is no longer files.go's. The row is not
//     owed here at all any more: ADR-0068 imports comics, so bookorbit.go's item
//     pass maps each one to a 'comic_issue' and store lands the row before this
//     walk runs. What this pass does not do is give an issue a FILE — a comic
//     gets no card kept, so it never reaches `imported` and never reaches this
//     walk — and that is ADR-0068's budget rather than an omission, since the
//     three card-fed passes are exactly what its "Zero extra HTTP" declined to
//     widen.
//
// # A NON-CONTENT FILE IS NOT A media_file ROW
//
// A BookOrbit book's `files[]` can hold a cover.jpg and a metadata.opf beside
// its epub — `book_files_role_chk` is
// `role in ('content','cover','metadata','supplement')` (db/schema/books.ts:92).
// Only 'content' (and the card's synthetic 'primary' relabel) become rows, via
// bookorbit.RoleCarriesContent, because work.have_count is COUNT(media_file)
// and a cover counted as a held file is a wrong number on a screen. The cost is
// stated where it lands: a book whose card carries ONLY decoration gets an
// empty FileSet, and store's writer gives an item with no files no edition at
// all — so it stays Ebooks, which is mediaTypeOf's documented answer for a book
// with no editions and the honest one for a book with no bytes.

// bookOrbitFilePathSurrogate mints media_file.path from a BookFileRef id.
//
// ⚠️ IT TAKES THE ID AND NOTHING ELSE, and the parameter list is the guarantee —
// kavitaFilePathSurrogate's rule, and it matters more here because BookOrbit
// DOES publish real paths: `bookFiles.absolutePath` is a NOT NULL varchar(4096)
// and BookDetailFile.absolutePath puts it on the wire. The card does not carry
// it, this function could not emit it if it did, and internal/store's
// validateReplicaPath is the backstop if some future adapter tries.
//
// The shape is `bookorbit:bookfile:<BookFileRef.id>` — source, upstream entity,
// upstream id — matching `kavita:mangafile:<id>` member for member. It is
// stable across imports (book_files.id is a serial primary key), unique within
// one instance (all ux_file_path asks), and it says what it is when a human
// reads it out of the database.
func bookOrbitFilePathSurrogate(fileID int64) string {
	return "bookorbit:bookfile:" + strconv.FormatInt(fileID, 10)
}

// bookOrbitEditionFormat maps the PRIMARY FILE'S format token onto
// edition.format's CHECK vocabulary.
//
// # The input is remote_subtype, and the classification is BookOrbit's own
//
// The token arrives on FileRequest.RemoteSubtype, written verbatim by mapBook
// from Book.PrimaryFile().Format. It is classified by bookorbit.MediaKindOf —
// getBookMediaKind transcribed — rather than by a second format table here,
// because a book is only imported at all if that same function called it prose.
// Two tables would be free to disagree, and the disagreement would show up as a
// book that imported as prose and then claimed a comic's medium.
//
// # The mapping, member by member
//
//   - audiobook (m4b, mp3, m4a, opus, ogg, flac) → 'audiobook'. This is the
//     line the Audiobooks screen depends on.
//   - ebook, and 'pdf' is split out of it: BookOrbit calls a PDF an ebook (its
//     kind vocabulary has no pdf member) and edition.format has BOTH 'ebook' and
//     'pdf', so the more specific member is used where the token supports it.
//     Everything else BookOrbit calls an ebook — epub, kepub, mobi, azw3, azw,
//     fb2 — is 'ebook'.
//   - comic → NULL, and it is unreachable rather than unhandled: StreamItems
//     skips and counts comics, so no comic-format book has a work for an edition
//     to hang on. Should one ever arrive, claiming 'cbz' for an edition of a
//     'book' work would be UsArr asserting a medium its own import path says it
//     did not accept.
//   - unknown (an absent or blank token) → NULL, which the column allows and
//     the CHECK still guards against a misspelling.
//
// # Why the primary file decides, and what that costs
//
// ⚠️ A BOOK HOLDING AN .EPUB AND AN .M4B GETS ONE EDITION, WHICHEVER FILE
// BOOKORBIT NOMINATED, and it will therefore render as Audiobooks when the m4b
// is primary. That is the OPPOSITE side from the one mediaTypeOf prefers — its
// comment argues Ebooks keeps a mixed work's row honest because the Ebooks
// format list is longer. The disagreement is real and it is not resolvable at
// this grain: one edition has one format, and the alternative shapes are worse.
//
// Deriving the format from unanimity across the content files instead would
// give NULL for the mixed book — which lands it in Ebooks, agreeing with
// mediaTypeOf — but it would also make UsArr's answer diverge from BookOrbit's
// own primaryMediaKind for the same book, and it would put the ebook/audiobook
// decision on a rule the upstream does not use. The honest fix is an edition
// PER FORMAT rather than a better guess at one, and the seam for it is exactly
// where store.primaryEditionID says it is: `edition` is a table this pass
// writes one row of, library_member is already edition-grained, and nothing in
// the schema has to change. Until then the medium UsArr claims is the medium
// BookOrbit claims, which is a defensible sentence; a medium neither of them
// claims is not.
func bookOrbitEditionFormat(remoteSubtype string) sql.NullString {
	switch bookorbit.MediaKindOf(remoteSubtype) {
	case bookorbit.MediaKindAudiobook:
		return sql.NullString{String: "audiobook", Valid: true}
	case bookorbit.MediaKindEbook:
		if bookorbit.NormalizeFormat(remoteSubtype) == "pdf" {
			return sql.NullString{String: "pdf", Valid: true}
		}
		return sql.NullString{String: "ebook", Valid: true}
	case bookorbit.MediaKindComic, bookorbit.MediaKindUnknown:
		return sql.NullString{}
	}
	return sql.NullString{}
}

// fileSetFromCard projects one kept card's files onto the schema.
//
// AN EMPTY SET IS DELIVERED, NOT SKIPPED — files.go's rule, and it has the same
// teeth here: "this book has no files" is what removes the rows of a book whose
// bytes were deleted upstream, and skipping it is how a deleted file counts
// forever.
func fileSetFromCard(r FileRequest, c bookOrbitCard) store.FileSet {
	set := store.FileSet{RemoteKind: r.RemoteKind, RemoteID: r.RemoteID}

	// seen guards the duplicate ux_file_path would otherwise take as an update
	// of the first row. BookOrbit should not report one file twice on one card —
	// findCards selects book_files by primary key — but the set the store
	// reconciles against and the rows actually written must not be able to
	// disagree, which is the same guard fileSetFromVolumes keeps.
	seen := map[int64]bool{}

	for _, f := range c.Files {
		if f.ID == 0 || seen[f.ID] {
			// book_files.id is a serial primary key, so 0 is not a file id. A
			// row keyed on it would collide with every other unidentified file.
			continue
		}
		if !bookorbit.RoleCarriesContent(f.Role) {
			// A cover, a metadata sidecar or a supplement. See the file header:
			// not a file the reader holds, and not a unit of have_count.
			continue
		}
		seen[f.ID] = true
		set.Files = append(set.Files, store.MediaFileRow{
			Path:         bookOrbitFilePathSurrogate(f.ID),
			RemoteFileID: strconv.FormatInt(f.ID, 10),
			SizeBytes:    f.SizeBytes,
			// DateAdded is deliberately absent — the card carries no per-file
			// timestamp. See the file header.
		})
	}

	set.EditionFormat = bookOrbitEditionFormat(r.RemoteSubtype)
	return set
}

// StreamFiles hands fn one FileSet per requested item whose card the walk kept.
//
// THE SECOND RETURN IS ALWAYS EMPTY, and that is the honest value rather than a
// stub. FileWalkFailure exists because a Kavita series whose GET failed had to
// stop being indistinguishable from one that walked clean and found nothing;
// this pass issues no GET, so the state it describes cannot occur. A request
// whose card is missing is NOT one of them — nothing was read and nothing
// failed, which is why it is a skip and not a failure (see BookOrbitSource.card).
//
// A PER-ITEM FAILURE CANNOT HAPPEN, but the invariant it protected still holds
// by construction: an item this pass does not deliver keeps the file rows it
// already had, because delivery is the only thing that can remove a row.
func (s *BookOrbitSource) StreamFiles(
	ctx context.Context, reqs []FileRequest, fn func(store.FileSet) error,
) (int, []FileWalkFailure, error) {
	var n int
	for _, r := range reqs {
		if err := ctx.Err(); err != nil {
			return n, nil, fmt.Errorf("bookorbit: walk files: %w", err)
		}
		c, ok := s.card(r.RemoteKind, r.RemoteID)
		if !ok {
			continue
		}
		n++
		if err := fn(fileSetFromCard(r, c)); err != nil {
			return n, nil, err
		}
	}
	return n, nil, nil
}

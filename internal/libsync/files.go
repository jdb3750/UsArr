package libsync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jdb3750/UsArr/internal/kavita"
	"github.com/jdb3750/UsArr/internal/store"
)

// The Kavita file walk: how GET /api/Series/volumes becomes media_file rows and
// one edition row per series.
//
// # It is ONE request per series, and that is measured
//
// A live probe against the owner's Kavita 0.9.0.2 (151 series) read volumes →
// chapters → files from this single endpoint, with `files[]` NON-EMPTY on 90 of
// 90 sampled chapters and null on none. So the walk descends three levels of a
// response it already has rather than issuing a call per volume or per chapter.
//
// THAT IS ALSO WHY THERE IS NO TOKEN BUCKET AND NO CONCURRENCY HERE, and the
// absence is a decision rather than an oversight. The same probe measured a
// MEDIAN 4 ms per call: ~0.6 s serially across the whole library, on a phase-C
// pass nobody is waiting on. Pacing machinery would be an answer to a problem
// the measurement says does not exist, and it would have to be maintained. The
// seam if a much larger library ever measures slow is this loop and nothing
// above it — exactly as credits.go says of its own.
//
// # What it writes, and what it deliberately does not
//
// media_file gets work_id, service_instance_id, path (the SURROGATE — see
// internal/store/files.go's header), remote_file_id, size_bytes and date_added.
// content_key and provenance_id stay NULL and the store's header says why.
//
// work_comic_issue is NOT written here. A chapter is a comic ISSUE and it has a
// work row's worth of facts in this same response, but landing it means a
// second work per chapter, its own identity resolution, its own search
// documents and its own membership — a slice of its own, not a rider on this
// one. What that costs today is that `missing` (contiguity, ARCHITECTURE §6.1's
// only always-honest completeness number) has nothing to compute from, and the
// rollup's blob carries no `missing` key as a result.

// FileRequest names one already-imported item to walk files for.
//
// RemoteSubtype rides along because THE EDITION COSTS ZERO EXTRA HTTP ONLY IF IT
// DOES: `SeriesDto.format` is on the item stream and already stored as
// service_item_link.remote_subtype, so carrying it forward from the batch that
// wrote it means the edition's format needs no second read. Re-reading it from
// the database would be a query per series to recover a value the importer had
// in hand.
//
// ⚠️ IT CARRIES NO `Kind`, unlike CreditRequest, and the asymmetry is a fact
// about the two passes rather than an oversight. A credit's ROLE depends on the
// work's kind — Kavita's `writers` array is a comic's 'writer' and a book's
// 'author' — while nothing in the file walk branches on kind: the edition's
// format comes from the upstream's own format plus the file extension, and a
// media_file row is the same row whatever the work is.
type FileRequest struct {
	RemoteKind string
	RemoteID   string

	// RemoteSubtype is the upstream's own sub-classification, verbatim — for
	// Kavita, MangaFormat rendered as a decimal string (§6.5 rule 3).
	RemoteSubtype string
}

// FileSource is the second OPTIONAL half of Source, beside CreditSource and for
// its reasons.
//
// A catalogue source that reports no files does not implement it, and the
// importer skips the whole phase rather than making every adapter author decide
// what a no-op walk means. That is principle 3 — "every feature degrades
// honestly when a service is absent" — applied to an adapter: Prowlarr has no
// files to walk and Sonarr's are a different shape entirely, so neither is
// forced to answer a question it has no answer for.
type FileSource interface {
	// StreamFiles hands fn one FileSet per item it could read, and returns HOW
	// MANY IT HANDED OVER — on failure too, on StreamItems's partial-delivery
	// terms: the calls to fn happened and their effects stand.
	//
	// The second return is the items it could NOT read, one entry each. It is
	// separate from the error because these are not the pass's failure: the
	// pass continues past them and returns nil. See FileWalkFailure.
	StreamFiles(
		ctx context.Context, reqs []FileRequest, fn func(store.FileSet) error,
	) (int, []FileWalkFailure, error)
}

// FileWalkFailure is one item whose file walk failed and was dropped.
//
// ⚠️ IT EXISTS BECAUSE THE DROP USED TO BE SILENT, AND SILENCE HERE IS A
// FALSEHOOD ON A SCREEN. A series whose walk failed keeps the file rows it had
// (StreamFiles's doc comment says why, and that half does not change), so a
// FIRST import leaves it with none — and no count, no blob, and http-api.md
// §1.4.1's honest "not counted yet" on the item. That rendering is only honest
// if somebody can find out WHY it is not counted. Before this type, "walked and
// found nothing" and "could not be walked" were indistinguishable everywhere
// downstream, which is the same defect class as rendering an absent blob as
// "none held".
//
// ⚠️ Reason IS A CLOSED VOCABULARY AND CARRIES NO UPSTREAM TEXT. See
// walkFailureReason: the reason a Kavita gave in a response body never reaches
// this struct, so it can never reach sync_report.detail or a log line.
type FileWalkFailure struct {
	RemoteKind string
	RemoteID   string

	// Reason is one of the tokens walkFailureReason returns.
	Reason string

	// Status is the upstream HTTP status when there was one, and 0 when the
	// failure happened before a status existed (a timeout, a transport error, a
	// refused decode). A number is not free text.
	Status int
}

// kavitaFilePathSurrogate mints media_file.path from a MangaFileDto.
//
// ⚠️ IT TAKES THE ID AND NOTHING ELSE, and the parameter list is the guarantee:
// this function CANNOT emit a host filesystem path because it is never given
// one. The owner decided on 2026-08-18 that media_file.path carries an opaque
// surrogate rather than Kavita's `filePath`, and internal/store/files.go's
// header carries the full reasoning plus validateReplicaPath, the guard that
// catches a future adapter doing otherwise.
//
// The shape is `kavita:mangafile:<MangaFileDto.id>` — source, upstream entity,
// upstream id. It is stable across imports (Kavita's MangaFile row id is), it
// is unique within one instance (which is all ux_file_path asks), and it says
// what it is when a human reads it out of the database.
func kavitaFilePathSurrogate(fileID int32) string {
	return "kavita:mangafile:" + strconv.FormatInt(int64(fileID), 10)
}

// kavitaEditionFormat maps the SERIES' MangaFormat onto edition.format's CHECK
// vocabulary, using the walked files' extensions to settle the one case the
// enum cannot.
//
// ⚠️ THE ARCHIVE CASE IS AMBIGUOUS AND THE EXTENSION IS WHAT DECIDES IT.
// MangaFormat.Archive (1) covers BOTH of edition.format's archive members —
// 'cbz' (a zip) and 'cbr' (a rar) — so the enum alone cannot choose, and
// picking one would be a coin flip stored as a fact. The walk has the answer
// already: MangaFileDto.extension is the file's own extension with its leading
// dot. So:
//
//   - every walked file agreeing on a zip extension (.cbz, .zip) → 'cbz';
//   - every walked file agreeing on a rar extension (.cbr, .rar) → 'cbr';
//   - anything else — a mixed series, an unrecognised extension, no files —
//     → NULL, which the column allows and the CHECK still guards against a
//     misspelling.
//
// NULL IS THE RIGHT ANSWER FOR A MIXED SERIES rather than "whichever came
// first": edition.format describes the edition, and a series held half as CBZ
// and half as CBR does not have one archive format. schema.md §2 says the
// column carries the MEDIUM, and "unknown medium" is a medium claim UsArr can
// stand behind where "cbz, probably" is not.
//
// The other members are unambiguous: Epub → 'ebook', Pdf → 'pdf'. Image (loose
// image files) and Unknown map to NULL because the vocabulary has no member for
// either — 'comic' is a medium (a printed comic), not "a directory of JPEGs",
// and reusing it would make the Ebooks/Audiobooks-style format routing wrong for
// the one type it exists to serve.
func kavitaEditionFormat(remoteSubtype string, extensions []string) sql.NullString {
	n, err := strconv.ParseInt(strings.TrimSpace(remoteSubtype), 10, 32)
	if err != nil {
		// A subtype this adapter did not write. Not a format claim.
		return sql.NullString{}
	}
	switch kavita.MangaFormat(n) {
	case kavita.MangaFormatEpub:
		return sql.NullString{String: "ebook", Valid: true}
	case kavita.MangaFormatPdf:
		return sql.NullString{String: "pdf", Valid: true}
	case kavita.MangaFormatArchive:
		return archiveFormatFromExtensions(extensions)
	case kavita.MangaFormatImage, kavita.MangaFormatUnknown:
		return sql.NullString{}
	}
	return sql.NullString{}
}

// archiveFormatFromExtensions is the cbz/cbr decision. See kavitaEditionFormat.
func archiveFormatFromExtensions(extensions []string) sql.NullString {
	var decided string
	for _, raw := range extensions {
		var member string
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case ".cbz", ".zip":
			member = "cbz"
		case ".cbr", ".rar":
			member = "cbr"
		default:
			// An extension outside the two families — .7z, .tar, or "" from an
			// upstream that did not report one. It makes the series undecided
			// rather than being ignored: ignoring it would let one .cbz decide
			// a series that is mostly something else.
			return sql.NullString{}
		}
		if decided == "" {
			decided = member
			continue
		}
		if decided != member {
			return sql.NullString{}
		}
	}
	if decided == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: decided, Valid: true}
}

// StreamFiles walks each requested series' volumes and hands its files to fn.
//
// ONE GET PER SERIES, SEQUENTIALLY, for the reason in the file header: it is
// measured at ~4 ms and the whole pass at ~0.6 s.
//
// A PER-SERIES FAILURE IS DROPPED, NOT FATAL, on StreamCredits's terms and with
// one extra consequence worth naming: a series whose walk fails keeps the file
// rows it already had. That is the right side of the trade — the alternative,
// treating a failed read as "no files", would delete every row for that series
// through the store's reconciliation and drive its count to zero. A failed read
// is not evidence of an empty library. NOTHING BELOW CHANGES THAT: a dropped
// series is still not delivered to fn, and delivery is the only thing that can
// remove a row.
//
// ⚠️ THE DROP IS RECORDED, and it did not used to be. This function returned
// `(int, error)`, a failed series took a bare `continue`, and the fact left no
// counter, no row and no log line — so downstream, a series nobody could read
// and a series that really holds nothing rendered identically. The dropped
// series now come back as []FileWalkFailure and are logged here; the importer
// owns what it does with them.
//
// Two conditions stop the pass, exactly as they stop the credit pass: a
// cancelled context, and an open circuit breaker. Both return the failures
// gathered so far, on the same partial-delivery terms as the count.
func (s *KavitaSource) StreamFiles(
	ctx context.Context, reqs []FileRequest, fn func(store.FileSet) error,
) (int, []FileWalkFailure, error) {
	var n int
	// Bounded by len(reqs), which is itself the importer's already-budgeted
	// per-import slice (streamAndApply's comment). Four short fields per FAILED
	// item, and the ordinary case is none of them.
	var failed []FileWalkFailure
	for _, r := range reqs {
		if err := ctx.Err(); err != nil {
			return n, failed, fmt.Errorf("kavita: walk files: %w", err)
		}
		id, err := strconv.ParseInt(r.RemoteID, 10, 32)
		if err != nil {
			// A remote id this adapter did not write; it cannot be a Kavita
			// series id. NOT a walk failure: nothing was attempted, and there is
			// no upstream to blame.
			continue
		}
		volumes, err := s.Client.SeriesVolumes(ctx, int32(id))
		if err != nil {
			if errors.Is(err, kavita.ErrBreakerOpen) {
				return n, failed, fmt.Errorf("kavita: walk files for series %s: %w", r.RemoteID, err)
			}
			// Dropped, and recorded. The series keeps the rows it has — see the
			// doc comment — but it is no longer indistinguishable from one that
			// walked clean and found nothing.
			reason, status := walkFailureReason(err)
			failed = append(failed, FileWalkFailure{
				RemoteKind: r.RemoteKind, RemoteID: r.RemoteID,
				Reason: reason, Status: status,
			})
			// WARN, not ERROR: the import continues and completes. It matches
			// the importer's own "container skipped" line, which is the nearest
			// neighbour — one recoverable per-row fault inside a pass that keeps
			// going. ⚠️ NO UPSTREAM TEXT, not even redacted: `reason` is a closed
			// vocabulary and `status` is an integer. ssrf.RedactText would be the
			// tool if a message rode along, and it is not the tool that makes
			// this safe — its own doc names the gap, "a bare secret that is not
			// inside a URL passes through untouched", and an upstream body
			// echoing a bare key is exactly R-08's Mylar3 shape.
			s.log().Warn("kavita: a series' file walk failed and was dropped",
				"remote_kind", r.RemoteKind, "remote_id", r.RemoteID,
				"reason", reason, "status", status)
			continue
		}
		set := fileSetFromVolumes(r, volumes)
		n++
		if err := fn(set); err != nil {
			return n, failed, err
		}
	}
	return n, failed, nil
}

// walkFailureReason classifies one failed SeriesVolumes call into a token and a
// status, and DELIBERATELY THROWS THE MESSAGE AWAY.
//
// ⚠️ THE RETURN VALUE IS THE REDACTION. kavita.APIError.Message is parsed out of
// an upstream response body; internal/kavita's `clean` already puts it through
// ssrf.RedactText, and that is still not enough to store: RedactText finds
// credentials INSIDE URLs, and states in its own doc that "a bare secret that is
// not inside a URL passes through untouched". sync_report.detail is durable and
// the row is what an operator pastes into a bug report, so the answer is not a
// better scrubber, it is to never carry the text — schema.md's column comment
// asks for redaction, and a closed vocabulary is redaction taken to its limit.
//
// The tokens are the sentinels' own names, because the sentinel is the
// classification internal/kavita already made and a second taxonomy here would
// be able to disagree with it.
func walkFailureReason(err error) (string, int) {
	var status int
	var apiErr *kavita.APIError
	if errors.As(err, &apiErr) {
		status = apiErr.Status
	}
	switch {
	case errors.Is(err, kavita.ErrUnauthorized):
		return "unauthorized", status
	case errors.Is(err, kavita.ErrForbidden):
		return "forbidden", status
	case errors.Is(err, kavita.ErrNotFound):
		return "not_found", status
	case errors.Is(err, kavita.ErrValidation):
		return "validation", status
	case errors.Is(err, kavita.ErrServer):
		return "server_error", status
	case errors.Is(err, kavita.ErrTimeout):
		return "timeout", status
	case errors.Is(err, kavita.ErrDecode):
		return "decode", status
	case errors.Is(err, kavita.ErrResponseTooLarge):
		return "response_too_large", status
	case errors.Is(err, kavita.ErrWrongService):
		return "wrong_service", status
	case errors.Is(err, kavita.ErrInvalidRequest):
		return "invalid_request", status
	case errors.Is(err, kavita.ErrUnexpectedStatus):
		return "unexpected_status", status
	}
	// A failure that matched no sentinel — a transport error the client did not
	// wrap, or a sentinel added upstream after this switch was written. It is
	// reported as unclassified rather than by quoting the error: an error string
	// is exactly the free text this function exists not to store.
	return "unknown", status
}

// fileSetFromVolumes projects one series' volume walk onto the schema.
//
// AN EMPTY SET IS DELIVERED, NOT SKIPPED — credits.go's rule, for the same
// reason and with more teeth: "this series has no files" is what removes the
// rows of a series whose chapters were deleted upstream, and skipping it is how
// a deleted chapter counts forever.
func fileSetFromVolumes(r FileRequest, volumes []kavita.VolumeDto) store.FileSet {
	set := store.FileSet{RemoteKind: r.RemoteKind, RemoteID: r.RemoteID}
	var extensions []string

	// seen guards the one duplicate the schema would reject anyway: Kavita can
	// report one MangaFile under two chapters of a split volume, and
	// ux_file_path would then take the second row as an update of the first. It
	// is skipped here instead so the reconciliation's "seen this pass" set and
	// the rows actually written cannot disagree.
	seen := map[int32]bool{}

	for _, v := range volumes {
		for _, ch := range v.Chapters {
			for _, f := range ch.Files {
				if f.ID == 0 || seen[f.ID] {
					// id 0 is .NET's zero value, not a file id. A row keyed on
					// it would collide with every other unidentified file.
					continue
				}
				seen[f.ID] = true
				set.Files = append(set.Files, store.MediaFileRow{
					Path:         kavitaFilePathSurrogate(f.ID),
					RemoteFileID: strconv.FormatInt(int64(f.ID), 10),
					SizeBytes:    f.Bytes,
					DateAdded:    f.Created.Time(),
				})
				extensions = append(extensions, f.Extension)
			}
		}
	}
	set.EditionFormat = kavitaEditionFormat(r.RemoteSubtype, extensions)
	return set
}

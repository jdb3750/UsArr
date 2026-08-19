package httpapi

import (
	"errors"
	"net/http"
)

// POST /api/v1/services/{id}/sync — the Services screen's "Run full sync now"
// (ARCHITECTURE.md §17.3, the action named for a *degraded, partial data* row).
//
// # Why this endpoint exists at all
//
// The catalogue full import had exactly one trigger before this: a bootstrap
// that runs on first connect and is gated on last_full_sync_at forever after.
// That gate is right for what it guards and it left the product with no way to
// import a second time — so any fix to what an import WRITES could never reach a
// row already imported. This is the re-run, not a backfill: a backfill repairs
// one field once, and the next fix needs another one.
//
// # What the response can and cannot observe, stated because it is easy to get
// wrong in the caller
//
// 202 means STARTED, and nothing more. It is not a claim that the import will
// finish, and this endpoint never learns whether it did — the goroutine outlives
// the response by minutes. A caller that reads 202 as "imported" will be wrong
// on every failure. The two observations that ARE available:
//
//   - `last_full_sync_at` on GET /api/v1/services/health, written on SUCCESS
//     ONLY. It advancing is the only positive evidence an import completed.
//   - This endpoint itself, asked again: 409 import_in_progress means it is
//     still running, and a 202 means it is not running any more — which,
//     combined with an unmoved last_full_sync_at, is exactly the readable form
//     of "started, then failed".
//
// A failed import leaves the batches it committed standing. "Started, then
// failed" therefore means a PARTIALLY updated catalogue, never a rolled-back
// one, and the honest rendering says so.
//
// EVERY NON-2XX HERE IS DECIDED BEFORE ANYTHING UPSTREAM IS TOUCHED. The kind
// check, the enabled check and the in-progress claim all complete synchronously
// inside StartImport, so a non-2xx from this endpoint means no import started
// FOR THIS CALL. That is not the same as "nothing is running": 409
// import_in_progress means another one is, and it says so.

// importStartResponse is the whole 202 body, field by field.
//
// THE ALLOWLIST IS THE POINT. service_instance carries an encrypted full-admin
// credential, a KEK id, a TLS pin and a base URL; none of them are here, and
// nothing is spread in from the row. Four fields, each chosen: the id and name
// so a client can match the response to the row it pressed, the kind because
// "which service was that" is the first question a log reader asks, and the
// status word so the three outcomes are readable without branching on the
// status code alone.
//
// THERE IS DELIBERATELY NO PROGRESS FIELD — no percentage, no item count, no
// estimate. The import has not read anything when this is written, so any such
// number would be invented, and a field that invites a progress bar the server
// cannot feed is worse than no field. Progress, where a client wants it, is the
// import.progress frame on the SSE stream, which carries real counts because it
// is published by the importer as batches commit.
type importStartResponse struct {
	Status     string `json:"status"`
	InstanceID int64  `json:"instance_id"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
}

func (s *Server) handleSyncService(w http.ResponseWriter, r *http.Request) error {
	a, _ := sessionFrom(r)
	id, err := pathInt64(r, "id")
	if err != nil {
		return err
	}
	// The scoped read is what makes "unknown instance" mean unknown TO THIS
	// USER, which is the answer multi-user needs and the one single-user gets
	// for free (principle 4).
	si, err := s.store.GetServiceInstance(r.Context(), storeScope(a), id)
	if err != nil {
		return notFoundOr(err, "service")
	}
	if !si.Enabled {
		return errStatus(http.StatusConflict, CodeServiceDisabled,
			"this service is turned off, so its catalogue is not being read").
			withAction("Enable the service")
	}
	// Refused HERE as well as in the implementation, because this is the check
	// that can be answered from SQLite alone. It keeps a Prowlarr from
	// travelling through a port whose job is imports.
	if serviceKinds[si.Kind] != roleLibrary {
		return errStatus(http.StatusConflict, CodeNotCatalogueSource,
			"this service is an indexer, not a catalogue source; there is no library here to import").
			withAction("Run a sync on a catalogue service")
	}
	if s.cfg.Imports == nil {
		return errStatus(http.StatusNotImplemented, CodeNotConfigured,
			"this build has no catalogue importer wired in")
	}

	if err := s.cfg.Imports.StartImport(si.ID); err != nil {
		return importStartError(err)
	}

	s.audit(r, "service.sync", "service_instance", si.ID, "started", "")
	writeJSON(w, http.StatusAccepted, importStartResponse{
		Status:     "started",
		InstanceID: si.ID,
		Kind:       si.Kind,
		Name:       si.Name,
	})
	return nil
}

// importStartError maps the port's sentinels onto the wire.
//
// The two 409s are separate codes rather than one `conflict`, because the two
// fixes are opposite — wait, versus press this on a different service — and a
// client that cannot tell them apart has to guess which sentence to show.
func importStartError(err error) error {
	switch {
	case errors.Is(err, ErrImportInProgress):
		return errStatus(http.StatusConflict, CodeImportInProgress,
			"an import is already running for this service; a second one was not started").
			withAction("Wait for the running import to finish")
	case errors.Is(err, ErrNotCatalogueSource):
		// ⚠️ THE WORDING IS DELIBERATELY WEAKER THAN "carries no catalogue", and
		// the reason has changed rather than gone away. It used to cover two
		// services — a Prowlarr with no catalogue at all, and a BookOrbit with
		// one that UsArr could not yet read — and the second half expired when
		// internal/libsync/bookorbit.go landed. What survives is the rule: this
		// sentence is shown for whatever kind has no adapter NEXT, and a screen
		// must not tell a user their books do not exist because UsArr cannot
		// read them.
		return errStatus(http.StatusConflict, CodeNotCatalogueSource,
			"UsArr has no catalogue reader for this service").
			withAction("Run a sync on a service UsArr can import from")
	}
	// Everything else is a configuration failure the user can act on — most
	// often a credential that will not open — and it is rendered verbatim for
	// the same reason §17.3's Problem column is.
	return errStatus(http.StatusInternalServerError, CodeInternal,
		"the import could not be started: "+redactText(err.Error())).
		withAction("Test connection").wrapping(err)
}

// roleLibrary is the role a catalogue source takes in serviceKinds. Named so the
// check above reads as a role check rather than as a string comparison that
// happens to work.
const roleLibrary = "library"

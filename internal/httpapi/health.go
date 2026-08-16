package httpapi

import (
	"net/http"
	"time"
)

// handleLive answers "is this process running".
//
// It touches nothing: no database, no lock, no session. A liveness probe that
// can fail for any reason other than "the process is gone" causes a restart
// loop when the thing it actually measured was slow, and a restart loop under a
// Docker healthcheck looks exactly like a crash.
func (s *Server) handleLive(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type readyResponse struct {
	Status              string `json:"status"`
	MigrationsApplied   bool   `json:"migrations_applied"`
	SchemaVersion       int64  `json:"schema_version"`
	ListenerAccepting   bool   `json:"listener_accepting"`
	SyncStateIsNotReady string `json:"sync_state_is_not_readiness"`
}

// handleReady answers "may this process be sent traffic".
//
// READINESS IS EXACTLY TWO THINGS: migrations applied, and the listener
// accepting. Nothing more (ARCHITECTURE.md §4.1).
//
// In particular, sync state is NOT a readiness signal, and the reason is
// operational rather than aesthetic: Docker's healthcheck defaults are
// start-period=0s with retries=3, so a first run that gates readiness on an
// initial library import gets killed and restarted before the import can finish,
// forever — a restart loop the user reads as a crash. The app can serve an empty
// or partial library perfectly well; sync progress is reported at
// /api/v1/system/sync and on the Services screen instead.
//
// The migrations half is recorded at construction rather than re-queried,
// because "nothing more" also means this handler does not touch SQLite.
func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	migrated := s.cfg.SchemaVersion > 0
	listening := s.listening.Load()

	resp := readyResponse{
		Status:              "ready",
		MigrationsApplied:   migrated,
		SchemaVersion:       s.cfg.SchemaVersion,
		ListenerAccepting:   listening,
		SyncStateIsNotReady: "sync progress is reported at /api/v1/system/sync, never here",
	}
	if migrated && listening {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp.Status = "not ready"
	writeJSON(w, http.StatusServiceUnavailable, resp)
}

type systemStatusResponse struct {
	AppName   string `json:"app_name"`
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"build_date,omitempty"`
	GoVersion string `json:"go_version,omitempty"`

	SchemaVersion int64  `json:"schema_version"`
	URLBase       string `json:"url_base"`

	StartedAt time.Time `json:"started_at"`
	Uptime    string    `json:"uptime"`

	// FrontendEmbedded is false in a binary built without `make web-build`.
	// Saying so is the honest degradation: the alternative is a blank page with
	// no error anywhere.
	FrontendEmbedded bool `json:"frontend_embedded"`
}

// handleSystemStatus reports UsArr's OWN version and build info — not any
// upstream service's. It is authenticated: build and schema versions are
// fingerprinting material and there is no reason for them to be anonymous.
func (s *Server) handleSystemStatus(w http.ResponseWriter, _ *http.Request) error {
	now := s.now()
	writeJSON(w, http.StatusOK, systemStatusResponse{
		AppName:          "UsArr",
		Version:          s.cfg.Build.Version,
		Commit:           s.cfg.Build.Commit,
		BuildDate:        s.cfg.Build.BuildDate,
		GoVersion:        s.cfg.Build.GoVersion,
		SchemaVersion:    s.cfg.SchemaVersion,
		URLBase:          s.cfg.URLBase,
		StartedAt:        s.startedAt,
		Uptime:           now.Sub(s.startedAt).Round(time.Second).String(),
		FrontendEmbedded: s.cfg.SPA != nil,
	})
	return nil
}

package httpapi

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/jdb3750/UsArr/internal/store"
)

// audit appends one audit row, best-effort.
//
// Best-effort is deliberate and narrow: the audit chain is tamper-EVIDENT, not
// a transactional participant, and failing a user's login because the audit
// insert lost a race would trade a real capability for a log line. The failure
// is logged loudly instead.
//
// metadataJSON must never carry a secret VALUE. Names, hosts, ids and reasons
// are fine; a key, a password or a token is not. Everything written here is
// already downstream of redactText.
func (s *Server) audit(r *http.Request, action, targetType string, targetID int64, result, metadataJSON string) {
	e := store.AuditEntry{
		TS:         s.now(),
		ActorIP:    ClientIP(r.Context()),
		Action:     action,
		TargetType: targetType,
		Result:     result,
	}
	if targetID > 0 {
		e.TargetID = strconv.FormatInt(targetID, 10)
	}
	if a, ok := sessionFrom(r); ok {
		e.ActorUserID = sql.NullInt64{Int64: a.User.ID, Valid: true}
	}
	if metadataJSON != "" {
		e.MetadataJSON = redactText(metadataJSON)
	}

	if _, err := s.store.AppendAudit(r.Context(), e); err != nil {
		s.log.Error("audit row not written",
			"action", action,
			"request", RequestLine(r.Context()),
			"err", redactText(err.Error()))
	}
}

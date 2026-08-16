package servarr

import (
	"context"
	"net/http"
)

// HealthCheckResult is the severity of one upstream health entry. Verified
// against the shipped Prowlarr OpenAPI spec (api/specs/prowlarr.json,
// components.schemas.HealthCheckResult): the enum is exactly these four.
type HealthCheckResult string

const (
	HealthOK      HealthCheckResult = "ok"
	HealthNotice  HealthCheckResult = "notice"
	HealthWarning HealthCheckResult = "warning"
	HealthError   HealthCheckResult = "error"
)

// HealthResource is one entry from GET /api/v1/health.
//
// Field set verified against components.schemas.HealthResource in the shipped
// spec; every string field there is nullable, which json.Unmarshal renders as
// the zero value.
type HealthResource struct {
	ID      int32             `json:"id"`
	Source  string            `json:"source,omitempty"`
	Type    HealthCheckResult `json:"type,omitempty"`
	Message string            `json:"message,omitempty"`
	WikiURL string            `json:"wikiUrl,omitempty"`
}

// Health calls GET /api/v1/health.
//
// This is the *Arr's OWN opinion of itself — "no download client is available",
// "indexers are unavailable due to failures", "branch is not a valid release
// branch". ARCHITECTURE.md §17.3 requires the Services screen to surface these,
// and it is the genuinely valuable half of that screen: today you visit five web
// UIs to notice that one of them is unhappy.
//
// The resource carries no credential, so unlike IndexerResource and
// DownloadClientResource it needs no Sanitize pass. The message text is
// upstream-authored and is rendered verbatim by the caller, which is the point;
// UsArr's redaction middleware still runs over it.
func (c *Client) Health(ctx context.Context) ([]HealthResource, error) {
	var out []HealthResource
	err := c.do(ctx, request{op: "Health", method: http.MethodGet, path: c.apiPath + "/health"}, &out)
	return out, err
}

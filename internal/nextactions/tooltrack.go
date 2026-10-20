package nextactions

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/analytics"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
)

// ToolRoute maps an API endpoint (method + path prefix, relative to /api/v1)
// to the tool slug recorded in `tool.used` meta.tool (§122 predicate feed).
// Match rule: the first entry whose method matches (or Method == "") and
// whose Prefix is a path prefix of the request path.
//
// Mapping convention: action endpoints (POST/calculate/generate/export)
// where the tool has one; the guide's detail view where reading the guide
// IS the tool's deliverable.
type ToolRoute struct {
	Method string
	Prefix string
	Slug   string
}

// ToolUsageRoutes is the canonical endpoint→tool map. Keep slugs aligned
// with the /tools/<slug> frontend routes and the §122 seed predicates.
var ToolUsageRoutes = []ToolRoute{
	{Method: "POST", Prefix: "/crs/calculate", Slug: "crs-calculator"},
	{Method: "POST", Prefix: "/assessment/submit", Slug: "readiness-assessment"},
	{Method: "POST", Prefix: "/cv-formatter/format", Slug: "cv-formatter"},
	{Method: "POST", Prefix: "/sop/generate", Slug: "sop-generator"},
	{Method: "POST", Prefix: "/credentials/", Slug: "credential-checker"},
	{Method: "POST", Prefix: "/jce/comparisons", Slug: "comparison-tool"},
	{Method: "POST", Prefix: "/jce/selections", Slug: "comparison-tool"},
	{Method: "POST", Prefix: "/community-locator", Slug: "community-locator"},
	{Method: "POST", Prefix: "/me/appointment-alerts", Slug: "embassy-alerter"},
	{Method: "POST", Prefix: "/me/visa-timelines", Slug: "processing-tracker"},
	{Method: "PATCH", Prefix: "/checklist/", Slug: "visa-checklist"},
	{Method: "GET", Prefix: "/calculator/cost-of-living/compare", Slug: "cost-of-living"},
	{Method: "GET", Prefix: "/pof/calculate", Slug: "pof-calculator"},
	{Method: "GET", Prefix: "/fx/convert", Slug: "currency-converter"},
	{Method: "GET", Prefix: "/remittance/calculate", Slug: "remittance"},
	{Method: "GET", Prefix: "/eca/guides/", Slug: "eca-guide"},
	{Method: "GET", Prefix: "/eca/compare", Slug: "eca-guide"},
	{Method: "GET", Prefix: "/attestation/", Slug: "attestation-guide"},
	{Method: "GET", Prefix: "/housing-guide/", Slug: "housing-guide"},
	{Method: "GET", Prefix: "/language-tests/", Slug: "language-test"},
	{Method: "GET", Prefix: "/credentials/", Slug: "credential-checker"},
	{Method: "GET", Prefix: "/checklist/", Slug: "visa-checklist"},
	{Method: "GET", Prefix: "/arrival-checklist/", Slug: "arrival-checklist"},
	{Method: "GET", Prefix: "/community-locator/", Slug: "community-locator"},
	{Method: "GET", Prefix: "/jce/comparisons/", Slug: "comparison-tool"},
	{Method: "GET", Prefix: "/scholarships/", Slug: "scholarships"},
	{Method: "GET", Prefix: "/processing-times", Slug: "processing-tracker"},
	{Method: "GET", Prefix: "/visa-routes/", Slug: "visa-routes"},
	{Method: "GET", Prefix: "/tax-calculator/", Slug: "tax-calculator"},
}

// apiPrefix is the mount point the middleware strips before matching.
const apiPrefix = "/api/v1"

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// TrackToolUsage is the §122 tool.used feed — emits one platform_event per
// successful request that maps to a tool action/engagement endpoint. Works
// for guests (session_id = client IP) and members (JTI + user_id) alike,
// so guest tool_use predicates and member tool_not_used predicates both
// resolve. Mounted once on the /api/v1 router.
func TrackToolUsage(tracker EventTracker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			if tracker == nil || rec.status >= 400 {
				return
			}
			path := r.URL.Path
			if rest, ok := strings.CutPrefix(path, apiPrefix); ok {
				path = rest
			}
			for _, rt := range ToolUsageRoutes {
				if rt.Method != "" && r.Method != rt.Method {
					continue
				}
				if !strings.HasPrefix(path, rt.Prefix) {
					continue
				}
				emitToolUsed(r, tracker, rt.Slug)
				return // first match wins
			}
		})
	}
}

func emitToolUsed(r *http.Request, tracker EventTracker, slug string) {
	var userID pgtype.UUID
	session := clientip.FromRequest(r)
	if u := auth.FromContext(r.Context()); u != nil {
		userID = u.UserID
		if u.SessionID != "" {
			session = u.SessionID
		}
	}
	_, _ = tracker.TrackEvent(r.Context(), analytics.TrackEventInput{
		EventType:  analytics.EventToolUsed,
		UserID:     userID,
		EntityType: "tool",
		Meta:       map[string]any{"tool": slug},
		SessionID:  session,
	})
}

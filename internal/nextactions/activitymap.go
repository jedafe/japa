package nextactions

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
)

// ActivityEmitFunc records an activity event for a user — the gamification
// v2 route-map bridge. Implemented in server.go over the wallet service
// (EmitActivityEvent + EnqueueProcessActivityEvent). Nil-safe by contract.
type ActivityEmitFunc func(ctx context.Context, userID pgtype.UUID, key, reference string)

// activityRoute maps an API endpoint (method + path prefix, relative to
// /api/v1) to an activity_key. A request emits when Method matches, the
// trimmed path starts with Prefix, and (when set) ends with Suffix.
// RefMode selects the dedupe reference:
//
//	"entity"      — first path segment after the prefix; requires a
//	                non-empty remainder (per-entity dedupe)
//	"root"        — emit only when the remainder is empty (exact prefix
//	                match); ref = UTC date (once per day)
//	"root_unique" — emit only on exact prefix match; ref is unique per
//	                request (each successful call counts — creates)
//	"path"        — full request path (per-endpoint dedupe)
//	"date"        — UTC date (counts distinct active days per user)
type activityRoute struct {
	Method   string
	Prefix   string
	Suffix   string
	MinDepth int // remainder must contain at least this many "/" separators
	Key      string
	RefMode  string
}

// ActivityRoutes is the gamification v2 catalogue route map — endpoints
// whose successful handling constitutes a badge-worthy activity (§proposal:
// meaningful actions, not raw page views). Prefixes are deliberately
// narrow: an overly-broad prefix would emit the key for unrelated
// sub-actions on the same route family (e.g. bookmarks counted as reads).
// Emissions dedupe through activity_events UNIQUE(activity_key, reference).
var ActivityRoutes = []activityRoute{
	// Content engagement — only the view-record endpoint counts (§14).
	{Method: http.MethodPost, Prefix: "/content/", Suffix: "/view", Key: "content.read", RefMode: "entity"},

	// Learning / exploration detail views — ref = entity slug|id.
	{Method: http.MethodGet, Prefix: "/jobs/", Key: "tool.jobs_board.used", RefMode: "entity"},
	{Method: http.MethodGet, Prefix: "/universities/", Key: "tool.universities.used", RefMode: "entity"},
	{Method: http.MethodGet, Prefix: "/scholarships/", Key: "tool.scholarships.used", RefMode: "entity"},
	{Method: http.MethodGet, Prefix: "/services/", Key: "tool.services_directory.used", RefMode: "entity"},
	{Method: http.MethodGet, Prefix: "/travel/", Key: "tool.travel.used", RefMode: "date"},

	// Wiki — entity-typed routes carry country/city in the path; every wiki
	// GET additionally counts as a page view (both keys emit on match).
	{Method: http.MethodGet, Prefix: "/wiki/e/country/", Key: "wiki.country_viewed", RefMode: "entity"},
	{Method: http.MethodGet, Prefix: "/wiki/e/city/", Key: "wiki.city_viewed", RefMode: "entity"},
	// List/index endpoints (/wiki/categories, /wiki/{space}) don't count —
	// a page read needs space+page or entity path depth.
	{Method: http.MethodGet, Prefix: "/wiki/", MinDepth: 1, Key: "wiki.page_viewed", RefMode: "path"},

	// Account & verification milestones.
	{Method: http.MethodGet, Prefix: "/auth/verify-email", Key: "email.verified", RefMode: "date"},

	// Community & journey actions — root-only so sub-action POSTs
	// (joins, comments, edits) never masquerade as creates. Verified
	// against internal/server/server.go registrations.
	{Method: http.MethodPost, Prefix: "/spaces", Key: "space.created", RefMode: "root_unique"},
	{Method: http.MethodPost, Prefix: "/scam-alerts", Key: "scam_report.submitted", RefMode: "root_unique"},
	{Method: http.MethodPost, Prefix: "/forum", Key: "forum.thread_created", RefMode: "root_unique"},
	{Method: http.MethodPost, Prefix: "/mentorship/request", Key: "mentorship.requested", RefMode: "root_unique"},
	{Method: http.MethodPost, Prefix: "/jce/comparisons", Key: "comparison.completed", RefMode: "root_unique"},
	{Method: http.MethodPost, Prefix: "/jce/comparisons/from-token", Key: "comparison.completed", RefMode: "unique"},

	// NOTE: POST /me/visa-timelines/{id}/milestones is intentionally absent —
	// the body rule below emits journey.milestone.* (or visa_timeline.milestone
	// for unmapped types) so a generic rule here would double-count.

	// NOTE: mentorship transitions emit at the service layer
	// (AcceptConnection/CompleteConnection) — events must fire for BOTH
	// parties with role-aware keys (mentor vs mentee), which a route map
	// can't express.

	// Targeted sub-endpoints.
	{Method: http.MethodPost, Prefix: "/thread/polls/", Suffix: "/vote", Key: "forum.poll_voted", RefMode: "entity"},
	{Method: http.MethodPost, Prefix: "/poll-options/", Suffix: "/vote", Key: "forum.poll_voted", RefMode: "entity"},
	{Method: http.MethodPost, Prefix: "/scholarships/", Suffix: "/save", Key: "scholarship.saved", RefMode: "entity"},
	{Method: http.MethodPost, Prefix: "/reviews/service_listing/", Key: "service.review_submitted", RefMode: "entity"},
	{Method: http.MethodPost, Prefix: "/reviews/service/", Key: "service.review_submitted", RefMode: "entity"},
	{Method: http.MethodPatch, Prefix: "/checklist/", Key: "checklist.item_checked", RefMode: "path"},
	{Method: http.MethodPost, Prefix: "/calendar/", Suffix: "/save", Key: "event.rsvp", RefMode: "entity"},
	{Method: http.MethodPost, Prefix: "/ai/chat", Key: "ai.conversation_started", RefMode: "date"},
}

// bodyActivityRoutes are endpoints whose activity key depends on the JSON
// request body (entity types, statuses, milestone kinds). The middleware
// buffers and replays the body for these routes only.
type bodyRouteRule struct {
	Field   string            // JSON field inspected
	KeyFor  map[string]string // field value → activity_key
	Default string            // key when field present but unmapped ("" = skip)
}

var bodyActivityRoutes = []struct {
	Method  string
	Prefix  string
	Rules   []bodyRouteRule
	RefMode string // "entity" = body entity_id; "path" = request path; "self" = once-per-user
}{
	// Typed saves — body {entity_type, entity_id}. Content-type bookmarks
	// (jobs/services saved via /content/{id}/bookmark) emit from
	// content.CreateBookmark instead, where the content type is known.
	{Method: http.MethodPost, Prefix: "/me/bookmarks", RefMode: "entity", Rules: []bodyRouteRule{
		{Field: "entity_type", KeyFor: map[string]string{
			"job": "job.saved", "job_listing": "job.saved",
			"scholarship": "scholarship.saved", "scholarship_guide": "scholarship.saved",
			"service": "service.saved", "service_listing": "service.saved",
		}},
	}},
	// Scholarship saved-status transitions — body {status}.
	{Method: http.MethodPatch, Prefix: "/scholarships/", RefMode: "path", Rules: []bodyRouteRule{
		{Field: "status", KeyFor: map[string]string{
			"applying": "scholarship.application_started", "applied": "scholarship.application_submitted",
		}},
	}},
	// Visa-timeline milestone kinds → normalized journey keys — body {milestone_type}.
	{Method: http.MethodPost, Prefix: "/me/visa-timelines/", RefMode: "path", Rules: []bodyRouteRule{
		{Field: "milestone_type", KeyFor: map[string]string{
			"applied":     "journey.milestone.application_submitted",
			"decision":    "journey.milestone.decision_received",
			"visa_issued": "journey.milestone.visa_approved",
			"arrived":     "journey.milestone.arrived",
		}, Default: "visa_timeline.milestone"},
	}},
	// Profile milestones — once-per-user events when a field is set — body fields.
	{Method: http.MethodPatch, Prefix: "/me/profile", RefMode: "self", Rules: []bodyRouteRule{
		{Field: "avatar_url", KeyFor: map[string]string{}, Default: "profile.photo_added"},
		{Field: "avatar_media_id", KeyFor: map[string]string{}, Default: "profile.photo_added"},
		{Field: "bio", KeyFor: map[string]string{}, Default: "profile.bio_added"},
		{Field: "destination_country", KeyFor: map[string]string{}, Default: "profile.destination_set"},
	}},
	// Poll creation rides on the thread-create payload — body {poll}. Each
	// poll counts (Poll Master ×3) so the ref must be unique per request.
	{Method: http.MethodPost, Prefix: "/forum", RefMode: "unique", Rules: []bodyRouteRule{
		{Field: "poll", KeyFor: map[string]string{}, Default: "forum.poll_created"},
	}},
}

// emitBodyActivities inspects a buffered JSON body and emits each matched
// rule at most once per request.
func emitBodyActivities(ctx context.Context, emit ActivityEmitFunc, userID pgtype.UUID, path, method string, body []byte) {
	if len(body) == 0 || len(body) > 1<<20 {
		return
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return
	}
	for _, br := range bodyActivityRoutes {
		if br.Method != "" && method != br.Method {
			continue
		}
		if !strings.HasPrefix(path, br.Prefix) {
			continue
		}
		for _, rule := range br.Rules {
			v, ok := parsed[rule.Field]
			if !ok || v == nil || v == "" {
				continue
			}
			key := rule.Default
			if s, isStr := v.(string); isStr {
				if k, hit := rule.KeyFor[s]; hit {
					key = k
				}
			} else {
				key = rule.Default // non-string field present (e.g. poll object)
			}
			if key == "" {
				continue
			}
			var ref string
			switch br.RefMode {
			case "entity":
				id, _ := parsed["entity_id"].(string)
				if id == "" {
					continue
				}
				ref = key + ":" + id
			case "path":
				ref = key + ":" + path
			case "unique":
				ref = key + ":" + path + ":" + time.Now().UTC().Format("20060102150405.000000000")
			default: // "self" — once per user per key
				ref = key + ":self"
			}
			emit(ctx, userID, key, ref)
		}
	}
}

// TrackActivityUsage emits activity_events for authenticated users on
// matched routes, after the handler succeeds (<400). It sits next to
// TrackToolUsage on the /api/v1 router: the tool slug map doubles as the
// tool.<slug>.used emitter source.
func TrackActivityUsage(emit ActivityEmitFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Buffer and replay the body only when the route needs body
			// inspection — cheap enough to do for all mutating requests.
			var body []byte
			if r.Method == http.MethodPost || r.Method == http.MethodPatch || r.Method == http.MethodPut {
				if r.Body != nil {
					body, _ = io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
					r.Body = io.NopCloser(bytes.NewReader(body))
				}
			}

			// WrapResponseWriter preserves optional interfaces (Flusher,
			// Hijacker, Pusher) — required for SSE (/ai/chat) and WS routes.
			rec := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(rec, r)

			if emit == nil || rec.Status() >= 400 {
				return
			}

			u := auth.FromContext(r.Context())
			if u == nil || !u.UserID.Valid {
				return // guests earn no badges
			}

			path := strings.TrimPrefix(r.URL.Path, apiPrefix)

			emitBodyActivities(r.Context(), emit, u.UserID, path, r.Method, body)

			// Tool catalogue — the §122 route map doubles as the activity
			// source: GET detail endpoints dedupe per entity; action
			// endpoints dedupe per day.
			for _, rt := range ToolUsageRoutes {
				if rt.Method != "" && r.Method != rt.Method {
					continue
				}
				if !strings.HasPrefix(path, rt.Prefix) {
					continue
				}
				rest := strings.Trim(strings.TrimPrefix(path, rt.Prefix), "/")
				ref := "tool:" + rt.Slug + ":"
				if rest != "" {
					ref += strings.SplitN(rest, "/", 2)[0]
				} else {
					ref += time.Now().UTC().Format("2006-01-02")
				}
				emit(r.Context(), u.UserID, "tool."+strings.ReplaceAll(rt.Slug, "-", "_")+".used", ref)
			}

			// Every matching catalogue rule emits — a request can be both a
			// tool use and a badge-worthy action (e.g. POST /jce/comparisons
			// → tool.comparison_tool.used + comparison.completed), and
			// specific + generic keys can coexist (wiki country view + page
			// view).
			for _, rt := range ActivityRoutes {
				if rt.Method != "" && r.Method != rt.Method {
					continue
				}
				if !strings.HasPrefix(path, rt.Prefix) {
					continue
				}
				if rt.Suffix != "" && !strings.HasSuffix(path, rt.Suffix) {
					continue
				}
				if rt.MinDepth > 0 {
					rest := strings.Trim(strings.TrimPrefix(path, rt.Prefix), "/")
					if strings.Count(rest, "/") < rt.MinDepth {
						continue
					}
				}

				var ref string
				switch rt.RefMode {
				case "entity":
					rest := strings.Trim(strings.TrimPrefix(path, rt.Prefix), "/")
					if rest == "" {
						continue // no entity in path — not a detail hit
					}
					ref = rt.Key + ":" + strings.SplitN(rest, "/", 2)[0]
				case "root":
					if rest := strings.Trim(strings.TrimPrefix(path, rt.Prefix), "/"); rest != "" {
						continue
					}
					ref = rt.Key + ":" + time.Now().UTC().Format("2006-01-02")
				case "root_unique":
					if rest := strings.Trim(strings.TrimPrefix(path, rt.Prefix), "/"); rest != "" {
						continue
					}
					ref = rt.Key + ":" + path + ":" + time.Now().UTC().Format("20060102150405.000000000")
				case "unique":
					ref = rt.Key + ":" + path + ":" + time.Now().UTC().Format("20060102150405.000000000")
				case "path":
					ref = rt.Key + ":" + path
				default: // "date"
					ref = rt.Key + ":" + time.Now().UTC().Format("2006-01-02")
				}

				emit(r.Context(), u.UserID, rt.Key, ref)
			}
		})
	}
}

// (status capture uses middleware.NewWrapResponseWriter — see tooltrack.go.)

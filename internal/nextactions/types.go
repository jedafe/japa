// Package nextactions implements the Next-Action Suggestion Engine
// (Section 122, ADR-132). Admin-configured rules in next_action_rules are
// evaluated against a code-registered predicate vocabulary and live user
// state, then delivered through pull-render channels (popup, announcement
// bar, sidebar widget, in-article card) and push channels (notification,
// push, email via Section 35).
//
// Distinct from internal/recommendations (ADR-064a): recommendations is a
// stateless pull list; next-actions carries a per-user lifecycle
// (pending → shown → dismissed/snoozed/completed/expired) with frequency
// caps and proactive delivery.
//
// Ref: japaconnect-engineering/sections/122-next-action-suggestions.md
//
//	japaconnect-engineering/adr/ADR-132-next-action-suggestions.md
package nextactions

import (
	"github.com/jackc/pgx/v5/pgtype"
)

// Audience filters who a rule targets. Empty slices mean "no constraint".
type Audience struct {
	UserClass          []string `json:"user_class"`
	Surfaces           []string `json:"surfaces"`
	Archetype          []string `json:"archetype"`
	JapaStage          []string `json:"japa_stage"`
	DestinationCountry []string `json:"destination_country"`
	OriginCountry      []string `json:"origin_country"`
}

// ConditionGroup is the predicate DSL container: all[] is AND-semantics,
// any[] is OR-semantics. An empty group evaluates true.
type ConditionGroup struct {
	All []Predicate `json:"all"`
	Any []Predicate `json:"any"`
}

// Predicate is one condition clause. The Predicate field names the
// registered evaluator; the remaining fields are its arguments
// (only the ones relevant to that predicate are populated).
type Predicate struct {
	Predicate  string   `json:"predicate"`
	In         []string `json:"in,omitempty"`
	Eq         *bool    `json:"eq,omitempty"`
	Lt         *int     `json:"lt,omitempty"`
	Gt         *int     `json:"gt,omitempty"`
	Gte        *int     `json:"gte,omitempty"`
	Slug       string   `json:"slug,omitempty"`
	Type       string   `json:"type,omitempty"`
	Feature    string   `json:"feature,omitempty"`
	WithinDays int      `json:"within_days,omitempty"`
	Window     string   `json:"window,omitempty"` // "session" or "Nd"
}

// Payload is the renderable suggestion content.
type Payload struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	CTALabel string `json:"cta_label"`
	CTAURL   string `json:"cta_url"`
	Icon     string `json:"icon"`
}

// Frequency bounds how often a rule may surface to one user.
type Frequency struct {
	Mode               string `json:"mode"` // once | recurring
	MaxImpressions     int    `json:"max_impressions"`
	CooldownHours      int    `json:"cooldown_hours"`
	SnoozeDefaultHours int    `json:"snooze_default_hours"`
}

// Rule is the fully-decoded next_action_rules row.
type Rule struct {
	ID               pgtype.UUID
	Slug             string
	Name             string
	Description      string
	Enabled          bool
	Priority         int32
	Audience         Audience
	Conditions       ConditionGroup
	Payload          Payload
	Channels         []string
	CompletionSignal *ConditionGroup
	Frequency        Frequency
	ValidFrom        pgtype.Timestamptz
	ValidUntil       pgtype.Timestamptz
	CreatedBy        pgtype.UUID
	CreatedAt        pgtype.Timestamptz
	UpdatedAt        pgtype.Timestamptz
}

// Suggestion is the API response item — a render-ready suggestion.
type Suggestion struct {
	ID       string   `json:"id"`        // rule id
	ActionID string   `json:"action_id"` // user_next_actions id; "" for guests
	RuleSlug string   `json:"rule_slug"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	CTALabel string   `json:"cta_label"`
	CTAURL   string   `json:"cta_url"`
	Icon     string   `json:"icon"`
	Channels []string `json:"channels"`
	Priority int32    `json:"priority"`

	// upsell is internal bookkeeping (not serialized): set when the rule's
	// completion_signal keys on account_tier/user_class so guest impression
	// writes can feed the per-day upsell cap.
	upsell bool
}

// user_next_actions lifecycle states.
const (
	StatusPending   = "pending"
	StatusShown     = "shown"
	StatusDismissed = "dismissed"
	StatusSnoozed   = "snoozed"
	StatusCompleted = "completed"
	StatusExpired   = "expired"
)

// Channel vocabulary.
const (
	ChannelPopup           = "popup"
	ChannelAnnouncementBar = "announcement_bar"
	ChannelSidebar         = "sidebar"
	ChannelInArticle       = "in_article"
	ChannelNotification    = "notification"
	ChannelPush            = "push"
	ChannelEmail           = "email"
)

// PullChannels render client-side on the pull path — available to guests.
var PullChannels = []string{ChannelPopup, ChannelAnnouncementBar, ChannelSidebar, ChannelInArticle}

// PushChannels are dispatched via the sweep + Section 35 — registered users only.
var PushChannels = []string{ChannelNotification, ChannelPush, ChannelEmail}

// User classes.
const (
	ClassGuest    = "guest"
	ClassRegular  = "regular"
	ClassPro      = "pro"
	ClassBusiness = "business"
)

// userActionState is a decoded user_next_actions row (or guest equivalent).
type userActionState struct {
	ActionID     pgtype.UUID
	RuleID       pgtype.UUID
	Status       string
	Impressions  int32
	LastShownAt  pgtype.Timestamptz
	SnoozedUntil pgtype.Timestamptz
}

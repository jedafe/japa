package sources

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Channel is the domain type for one entry point under an intel_sources row
// (entry URL + crawl method + entity pipeline). It is an alias for the
// sqlc-generated IntelSourceChannel so it can be passed interchangeably with
// the generated type while reading cleanly in downstream signatures
// (fetcher, extractor, scheduler, inbound handlers).
type Channel = gen.IntelSourceChannel

// ChannelID is the primary-key type for an intel_source_channels row.
type ChannelID = pgtype.UUID

// InboundEvent is the domain type for a push-payload row written by the
// inbound email/webhook endpoints before pipeline processing.
type InboundEvent = gen.IntelInboundEvent

// Pipeline selects which downstream processing path a channel's records
// take (migration 000681 CHECK: extract|curate|hybrid).
type Pipeline string

// Exported const values.
const (
	// PipelineExtract sends the page body to the LLM extractor; records go
	// through intel_records regardless of source trust.
	PipelineExtract Pipeline = "extract"
	// PipelineCurate promotes feed/itemized records directly; trust_level
	// decides auto-publish vs review queue.
	PipelineCurate Pipeline = "curate"
	// PipelineHybrid runs the curate phase per item and the extract phase
	// per page.
	PipelineHybrid Pipeline = "hybrid"
)

// IsValid reports whether p is a pipeline value the CHECK constraint allows.
func (p Pipeline) IsValid() bool {
	switch p {
	case PipelineExtract, PipelineCurate, PipelineHybrid:
		return true
	}

	return false
}

// pipelineRequiresEntityTypes reports whether the pipeline produces typed
// records that must be validated against channel.entity_types schemas —
// the extract and curate pipelines (spec: "channel.entity_types[] is the
// single source of truth for which schema validates each record").
func pipelineRequiresEntityTypes(p Pipeline) bool {
	return p == PipelineExtract || p == PipelineCurate
}

// RelevanceMode controls the per-item relevance gate on multi-niche sources
// (migration 000681 CHECK: open|filtered|custom).
type RelevanceMode string

// Exported const values.
const (
	// RelevanceOpen disables gating — every item is processed.
	RelevanceOpen RelevanceMode = "open"
	// RelevanceFiltered gates items on the source's category/topic defaults.
	RelevanceFiltered RelevanceMode = "filtered"
	// RelevanceCustom gates items on the channel's relevance_topics and
	// relevance_url_patterns.
	RelevanceCustom RelevanceMode = "custom"
)

// IsValid reports whether m is a relevance_mode value the CHECK constraint
// allows.
func (m RelevanceMode) IsValid() bool {
	switch m {
	case RelevanceOpen, RelevanceFiltered, RelevanceCustom:
		return true
	}

	return false
}

// ChannelOrigin records how a channel was created (migration 000681 CHECK:
// probe|admin|user).
type ChannelOrigin string

// Exported const values.
const (
	// ChannelOriginProbe marks channels discovered by the two-stage probe.
	ChannelOriginProbe ChannelOrigin = "probe"
	// ChannelOriginAdmin marks channels created by an admin (incl. backfill).
	ChannelOriginAdmin ChannelOrigin = "admin"
	// ChannelOriginUser marks channels proposed by an authenticated user.
	ChannelOriginUser ChannelOrigin = "user"
)

// IsValid reports whether o is an origin value the CHECK constraint allows.
func (o ChannelOrigin) IsValid() bool {
	switch o {
	case ChannelOriginProbe, ChannelOriginAdmin, ChannelOriginUser:
		return true
	}

	return false
}

// IsValid reports whether m is a crawl method the fetcher adapters or the
// inbound endpoints know how to handle.
func (m CrawlMethod) IsValid() bool {
	switch m {
	case CrawlMethodStatic, CrawlMethodRSS, CrawlMethodAPI, CrawlMethodPDF,
		CrawlMethodOCR, CrawlMethodManual,
		CrawlMethodRender, CrawlMethodJSONLD, CrawlMethodTabular,
		CrawlMethodSitemap, CrawlMethodICal, CrawlMethodGit, CrawlMethodSocial,
		CrawlMethodSearch, CrawlMethodWayback, CrawlMethodXHRSniff,
		CrawlMethodOneshot, CrawlMethodEmail, CrawlMethodWebhook,
		CrawlMethodSpider:
		return true
	}

	return false
}

// InboundMethod reports whether m is a push method (never scheduled —
// ListDueChannels excludes it and the inbound endpoints own ingestion).
func InboundMethod(m CrawlMethod) bool {
	return m == CrawlMethodEmail || m == CrawlMethodWebhook
}

// validateChannelCreate applies the domain rules for a new channel row after
// the service has filled defaults: known crawl_method/pipeline/relevance_mode/
// origin, a fetchable http(s) entry_url for pull methods (inbound channels
// only require a non-empty entry point — email/webhook entry URLs may be
// mailto: addresses or provider identifiers), and entity_types for the
// record-producing pipelines. allowedLiteralHost is the parent source's
// host[:port] for the same-host private-literal carve-out ("" = strict).
func validateChannelCreate(arg gen.CreateIntelSourceChannelParams, allowedLiteralHost string) error {
	if !arg.SourceID.Valid {
		return fmt.Errorf("sources: channel: %w: source_id", errpkg.ErrRequired)
	}
	if arg.Label == "" {
		return fmt.Errorf("sources: channel: %w: label", errpkg.ErrRequired)
	}

	method := CrawlMethod(arg.CrawlMethod)
	if !method.IsValid() {
		return fmt.Errorf("sources: channel: %w: unknown crawl_method %q", errpkg.ErrInvalid, arg.CrawlMethod)
	}

	if err := validateChannelEntryURL(arg.EntryUrl, method, allowedLiteralHost); err != nil {
		return err
	}

	pipeline := Pipeline(arg.Pipeline)
	if !pipeline.IsValid() {
		return fmt.Errorf("sources: channel: %w: unknown pipeline %q", errpkg.ErrInvalid, arg.Pipeline)
	}
	if pipelineRequiresEntityTypes(pipeline) && len(arg.EntityTypes) == 0 {
		return fmt.Errorf("sources: channel: %w: entity_types required for %s pipeline", errpkg.ErrInvalid, pipeline)
	}

	if !RelevanceMode(arg.RelevanceMode).IsValid() {
		return fmt.Errorf("sources: channel: %w: unknown relevance_mode %q", errpkg.ErrInvalid, arg.RelevanceMode)
	}
	if !ChannelOrigin(arg.Origin).IsValid() {
		return fmt.Errorf("sources: channel: %w: unknown origin %q", errpkg.ErrInvalid, arg.Origin)
	}

	return nil
}

// privateIPNets are the private/reserved ranges a channel entry_url host
// must not point at — the same range set as
// internal/linkpreview/ssrf.go and fetcher's safetransport privateNets
// (the import graph forces a third copy: sources cannot import fetcher —
// fetcher already imports sources — and linkpreview pulls internal/auth,
// which would close a cycle through internal/worker). The check applied
// here is a cheap net.ParseIP literal check at insert time; hostname
// resolution is deliberately NOT done — the fetcher's dial-time guard
// covers resolved names.
var privateIPNets = []net.IPNet{
	mustCIDR("127.0.0.0/8"),        // loopback
	mustCIDR("10.0.0.0/8"),         // private class A
	mustCIDR("172.16.0.0/12"),      // private class B
	mustCIDR("192.168.0.0/16"),     // private class C
	mustCIDR("169.254.0.0/16"),     // link-local
	mustCIDR("0.0.0.0/8"),          // current network
	mustCIDR("100.64.0.0/10"),      // shared address space (CGNAT)
	mustCIDR("192.0.0.0/24"),       // IETF protocol assignments
	mustCIDR("192.0.2.0/24"),       // TEST-NET-1
	mustCIDR("198.51.100.0/24"),    // TEST-NET-2
	mustCIDR("203.0.113.0/24"),     // TEST-NET-3
	mustCIDR("224.0.0.0/4"),        // IPv4 multicast
	mustCIDR("240.0.0.0/4"),        // reserved
	mustCIDR("255.255.255.255/32"), // broadcast
	mustCIDR("::1/128"),            // IPv6 loopback
	mustCIDR("fc00::/7"),           // IPv6 unique local
	mustCIDR("fe80::/10"),          // IPv6 link-local
	mustCIDR("ff00::/8"),           // IPv6 multicast
}

func mustCIDR(s string) net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(fmt.Sprintf("sources: invalid private CIDR %q: %v", s, err))
	}

	return *n
}

// IsPrivateIPLiteral reports whether host is an IP literal in a
// private/reserved range (loopback, RFC1918, link-local, CGNAT, ULA,
// multicast, unspecified, documentation/reserved). Non-literal hosts return
// false — name resolution is a dial-time concern (fetcher's guarded
// DialContext), not an insert-time one. Exported for the probe's
// discovered-URL checks (probe imports sources already).
func IsPrivateIPLiteral(host string) bool {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return false
	}

	return IsPrivateIP(ip)
}

// IsPrivateIP reports whether ip falls in any private/reserved range,
// normalizing IPv4-mapped IPv6 literals so they are caught by the IPv4 CIDRs.
func IsPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}

	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}

	for _, n := range privateIPNets {
		if n.Contains(ip) {
			return true
		}
	}

	return false
}

// validateChannelEntryURL enforces the entry point shape: pull methods must
// be fetchable http(s) URLs whose host is not a private/reserved IP literal
// (SSRF — a literal target would be crawled on schedule and its body
// persisted into intel_raw_documents); inbound methods accept any non-empty
// entry point (mailto:, provider address, webhook slug).
//
// allowedLiteralHost (host[:port] of the parent source_url, "" when
// unknown) carves out same-host literals: a channel on the source's own
// host is never an SSRF escalation beyond the already-registered target —
// and keeps dev/test environments (sources on 127.0.0.1) functional. A
// literal on any OTHER host is always rejected; names are left to the
// fetcher's dial-time guard.
func validateChannelEntryURL(entryURL string, method CrawlMethod, allowedLiteralHost string) error {
	if entryURL == "" {
		return fmt.Errorf("sources: channel: %w: entry_url", errpkg.ErrRequired)
	}
	if InboundMethod(method) {
		return nil
	}

	u, err := url.Parse(entryURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("sources: channel: %w: entry_url must be an http(s) URL", errpkg.ErrInvalid)
	}

	if IsPrivateIPLiteral(u.Hostname()) && !strings.EqualFold(u.Host, allowedLiteralHost) {
		return fmt.Errorf("sources: channel: %w: entry_url targets a private/reserved IP", errpkg.ErrInvalid)
	}

	return nil
}

// validateChannelUpdate applies the per-field rules for a COALESCE-style
// channel patch: only non-nil fields are checked, and trimmed values are
// written back so whitespace-only input cannot reach the database. The
// service performs the cross-field checks that need the existing row
// (e.g. pipeline → entity_types) separately.
//
// existingMethod is the channel's current crawl_method — needed to apply
// the relaxed entry_url rule to inbound channels (email/webhook accept
// non-URL entry points at create; a patch must not strand them). When the
// same patch changes crawl_method the incoming value wins.
func validateChannelUpdate(arg gen.UpdateIntelSourceChannelParams, existingMethod CrawlMethod) error {
	if arg.Label != nil {
		label := strings.TrimSpace(*arg.Label)
		if label == "" {
			return fmt.Errorf("sources: channel: %w: label must not be empty", errpkg.ErrInvalid)
		}
		*arg.Label = label
	}

	if arg.EntryUrl != nil {
		entryURL := strings.TrimSpace(*arg.EntryUrl)
		if entryURL == "" {
			return fmt.Errorf("sources: channel: %w: entry_url", errpkg.ErrRequired)
		}
		method := existingMethod
		if arg.CrawlMethod != nil {
			method = CrawlMethod(*arg.CrawlMethod)
		}
		if !InboundMethod(method) {
			u, err := url.Parse(entryURL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return fmt.Errorf("sources: channel: %w: entry_url must be an http(s) URL", errpkg.ErrInvalid)
			}
			if IsPrivateIPLiteral(u.Hostname()) {
				return fmt.Errorf("sources: channel: %w: entry_url targets a private/reserved IP", errpkg.ErrInvalid)
			}
		}
		*arg.EntryUrl = entryURL
	}

	if arg.CrawlMethod != nil && !CrawlMethod(*arg.CrawlMethod).IsValid() {
		return fmt.Errorf("sources: channel: %w: unknown crawl_method %q", errpkg.ErrInvalid, *arg.CrawlMethod)
	}

	if arg.Pipeline != nil && !Pipeline(*arg.Pipeline).IsValid() {
		return fmt.Errorf("sources: channel: %w: unknown pipeline %q", errpkg.ErrInvalid, *arg.Pipeline)
	}

	return nil
}

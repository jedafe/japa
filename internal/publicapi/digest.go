package publicapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/feed"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/visitor"
)

// Browser-extension digest (Section 47 v1.3) — one aggregate call per
// new-tab open. The key owner is the viewer; every section reads through
// the same public readers as the standalone endpoints.
//
// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md

// IntelReader is the narrow destination-intel seam the digest needs —
// latest promoted intel items for a space, filtered by the viewer's
// blocked intel sources (same as the radar followed feed). Satisfied by a
// wiring-layer adapter over the intel domain queries
// (ListVisaRoutesBySpaces + ListIntelJobsBySpaces), mirroring the radar
// corpus.
type IntelReader interface {
	ListRecentForSpace(ctx context.Context, viewerID, spaceID pgtype.UUID, limit int32) ([]IntelDigestItem, error)
}

// IntelDigestItem is the curated public DTO for a destination intel card.
type IntelDigestItem struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"` // "visa_route" | "job"
	Title     string `json:"title"`
	Summary   string `json:"summary,omitempty"`
	CreatedAt string `json:"created_at"`
}

// DigestFeedSection is one feed slice of the digest.
type DigestFeedSection struct {
	Items  []PublicFeedItem `json:"items"`
	Source string           `json:"source"`
}

// DigestSpaceRef is the compact space reference embedded in destination.
type DigestSpaceRef struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Level string `json:"level"`
}

// DigestDestination bundles the destination-space card data.
type DigestDestination struct {
	Space      DigestSpaceRef     `json:"space"`
	ScamAlerts []PublicScamReport `json:"scam_alerts"`
	Intel      []IntelDigestItem  `json:"intel"`
}

// DigestCommunity bundles the community feed sections.
type DigestCommunity struct {
	Popular []PublicFeedItem `json:"popular"`
}

// PublicDigest is the response shape for GET /public/me/digest.
type PublicDigest struct {
	ForYou      DigestFeedSection  `json:"for_you"`
	Destination *DigestDestination `json:"destination"`
	Community   DigestCommunity    `json:"community"`
	GeneratedAt time.Time          `json:"generated_at"`
	Cached      bool               `json:"cached"`
}

// SetIntelReader injects the destination-intel reader used by the digest.
func (s *Service) SetIntelReader(r IntelReader) { s.intelReader = r }

// digestViewerHash is the cache-key identity — the key owner's UUID hashed.
// The digest reads no member-scoped data (for_you is keyed on the owner
// directly), so retrieval scope hashes are not part of the key.
func digestViewerHash(viewerID pgtype.UUID) string {
	sum := sha256.Sum256(viewerID.Bytes[:])
	return hex.EncodeToString(sum[:])
}

// Digest serves GET /public/me/digest (api.read.digest). Responses cache at
// jc:api_digest:{viewer_hash} for api.digest.cache_ttl_seconds (default 120).
// Optional sections degrade to null/empty on failure — only the feed
// sections failing fails the request.
func (s *Service) Digest(ctx context.Context, viewerID pgtype.UUID) (*PublicDigest, error) {
	if s.feedReader == nil {
		return nil, ErrReaderUnconfigured
	}

	cacheKey := valkey.MustKey("api_digest:%s", digestViewerHash(viewerID))
	if s.cache != nil {
		if raw, err := s.cache.Get(ctx, cacheKey).Bytes(); err == nil {
			var cached PublicDigest
			if json.Unmarshal(raw, &cached) == nil {
				cached.Cached = true
				return &cached, nil
			}
		}
	}

	limit := int32(8)
	if s.settingsReader != nil {
		if v := s.settingsReader.GetIntOr(ctx, "api.digest.section_limit", 8); v > 0 {
			limit = int32(v)
		}
	}

	digest := &PublicDigest{GeneratedAt: time.Now().UTC()}

	// for_you — the same nil-VisitorContext call as MyFeed (geo
	// personalization skipped, consistent with /public/me/feed).
	items, err := s.feedReader.GetForYouFeed(ctx, viewerID, "", limit, 0, nil)
	if err != nil {
		return nil, err
	}
	digest.ForYou = DigestFeedSection{Items: mapFeedItems(items), Source: "for_you"}

	// community.popular — the explore-popular slice.
	popular, err := s.feedReader.GetFeed(ctx, "explore", nil, nil, nil, feed.SortTop, limit, 0, "", "")
	if err != nil {
		return nil, err
	}
	digest.Community = DigestCommunity{Popular: mapFeedItems(popular)}

	// destination — degrades to null when the owner has no resolvable
	// destination_country (current_geo_space_id is deliberately not a
	// fallback: it is where the user IS, not where they are going).
	digest.Destination = s.digestDestination(ctx, viewerID)

	if s.cache != nil {
		ttl := 120
		if s.settingsReader != nil {
			if v := s.settingsReader.GetIntOr(ctx, "api.digest.cache_ttl_seconds", 120); v > 0 {
				ttl = v
			}
		}
		if raw, err := json.Marshal(digest); err == nil {
			_ = s.cache.Set(ctx, cacheKey, raw, time.Duration(ttl)*time.Second).Err()
		}
	}

	return digest, nil
}

// digestDestination resolves the owner's destination_country custom field
// to a public+active country space and loads its scam/intel sections.
// Every failure inside degrades the section (or the whole destination) to
// null — a missing profile field is normal, not an error.
func (s *Service) digestDestination(ctx context.Context, viewerID pgtype.UUID) *DigestDestination {
	if s.queries == nil || s.moat == nil || s.moat.Spaces == nil {
		return nil
	}

	fields, err := s.queries.GetUserGeoProfileFields(ctx, viewerID)
	if err != nil {
		return nil
	}
	code := visitor.NormalizeCountry(fields.DestinationCountry)
	if code == "" {
		return nil
	}

	// GetPublicSpaceByGeoCode enforces visibility='public' AND
	// status='active' AND deleted_at IS NULL in SQL — non-public or
	// archived country spaces can never surface in the digest.
	space, err := s.moat.Spaces.GetPublicSpaceByGeoCode(ctx, code)
	if err != nil {
		return nil
	}

	dest := &DigestDestination{
		Space:      DigestSpaceRef{Slug: space.Slug, Name: space.Name, Level: space.Level},
		ScamAlerts: []PublicScamReport{},
		Intel:      []IntelDigestItem{},
	}

	// Scam alerts honour scam_alert.enabled inside the aggregate — the
	// route-level features.Gate does not apply here.
	if s.safetyReader != nil && s.safetyReader.Enabled(ctx) {
		if dtos, _, err := s.safetyReader.ListByOffset(ctx, space.ID, "", 3, 0); err == nil {
			for _, d := range dtos {
				dest.ScamAlerts = append(dest.ScamAlerts, mapPublicScamReport(d))
			}
		}
	}

	if s.intelReader != nil {
		if items, err := s.intelReader.ListRecentForSpace(ctx, viewerID, space.ID, 3); err == nil {
			dest.Intel = items
		}
	}

	return dest
}

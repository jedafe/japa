package sources

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// discovery.go — Phase-4 outbound-link source discovery (intel-channels
// spec §8.2). A link on a trusted page produces a CANDIDATE row only:
// status='proposed', enabled=false, trust_level 5, crawl_method manual.
// The row gains channels and trust only through the normal admin-approval
// → probe_verify path, so outbound discovery can never smuggle untrusted
// content into the pipeline.

// DiscoveredSourceInput is the internal proposal payload for a source
// discovered from an outbound link on a crawled page.
type DiscoveredSourceInput struct {
	// URL is the linked candidate page (normalized to an http(s) URL).
	URL string
	// Categories are inherited from the referring source — a scholarship
	// portal linked from a scholarship source is probably scholarship
	// content. At least one is required (same rule as user proposals).
	Categories []string
	// ReferrerName / ReferrerURL carry provenance into the description
	// shown in the admin review queue.
	ReferrerName string
	ReferrerURL  string
}

// ProposeDiscovered registers a source candidate discovered from an
// outbound link. It dedupes on the exact hostname (a host already in the
// registry under any status is not re-proposed — subdomain siblings are
// distinct candidates by design) and on the exact source_url, and
// reports created=false for both so callers can cap on successful
// inserts rather than attempts.
func (s *Service) ProposeDiscovered(ctx context.Context, in DiscoveredSourceInput) (Source, bool, error) {
	rawURL := strings.TrimSpace(in.URL)
	if rawURL == "" {
		return Source{}, false, fmt.Errorf("%w: url", errpkg.ErrRequired)
	}

	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}

	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Source{}, false, fmt.Errorf("sources: discovery: %w: invalid url", errpkg.ErrInvalid)
	}

	domain := strings.ToLower(u.Hostname())
	if domain == "" || !strings.Contains(domain, ".") || IsPrivateIPLiteral(domain) {
		return Source{}, false, fmt.Errorf("sources: discovery: %w: invalid domain", errpkg.ErrInvalid)
	}

	categories := NormalizeCategories(in.Categories)
	if len(categories) == 0 {
		return Source{}, false, fmt.Errorf("%w: categories", errpkg.ErrRequired)
	}

	if ok, err := s.store.DomainExists(ctx, domain); err != nil {
		return Source{}, false, fmt.Errorf("sources: discovery: check domain: %w", err)
	} else if ok {
		return Source{}, false, nil
	}

	if _, err := s.store.GetByURL(ctx, rawURL); err == nil {
		return Source{}, false, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Source{}, false, fmt.Errorf("sources: discovery: check existing url: %w", err)
	}

	name := domain
	desc := fmt.Sprintf("Discovered via outbound link on %s (%s)", in.ReferrerName, in.ReferrerURL)
	slug := s.uniqueSlug(ctx, slugify(domain))

	arg := gen.CreateIntelSourceParams{
		Name:               name,
		Domain:             domain,
		SourceUrl:          rawURL,
		Category:           categories[0],
		Categories:         categories,
		SourceType:         "auto",
		SourceKind:         string(SourceKindPublisher),
		TrustLevel:         5,
		CrawlMethod:        string(CrawlMethodManual),
		CrawlFrequency:     string(CrawlFrequencyManual),
		CrawlFrequencyMins: 0,
		RobotsPolicy:       []byte("{}"),
		ProviderConfig:     []byte("{}"),
		Enabled:            false,
		Slug:               &slug,
		Description:        &desc,
		Status:             "proposed",
		ProposedAt:         pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	src, err := s.store.Create(ctx, arg)
	if errors.Is(err, errpkg.ErrAlreadyExists) {
		return Source{}, false, nil
	}

	if err != nil {
		return Source{}, false, err
	}

	return src, true, nil
}

// SourceCategories returns the source's effective category list —
// exported for callers outside the package (the worker's discovery flow).
func SourceCategories(src Source) []string {
	return sourceCategories(src)
}

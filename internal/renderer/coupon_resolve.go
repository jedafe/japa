// Package renderer — coupon_embed and affiliate link render-time resolution.
//
// This file implements the P4 render-time resolution post-processing step
// (Section 55). After the TipTap SSR script renders placeholder HTML, the
// Go backend post-processes the output to:
//
//  1. Resolve each <div data-type="coupon-embed" data-coupon-id="..."> by
//     querying the coupon store and replacing the placeholder with the
//     resolved coupon widget HTML (or "Offer expired" if status != active
//     or past expires_at).
//  2. Rewrite <a> tags whose href matches a registered affiliate provider
//     domain, injecting tracking params (tracking_param=tracking_value).
//
// This allows expired coupons, changed codes, and rotated tracking parameters
// to take effect at render time without editing content (P4 architecture).
package renderer

import (
	"bytes"
	"context"
	"fmt"
	htmlpkg "html"
	"log/slog"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/net/html"
)

// CouponResolver resolves a coupon ID to its render-time representation.
// Implemented by coupons.Service. The tenantID scopes the lookup so a render
// request cannot resolve a coupon from another tenant (BE-LOW-001).
type CouponResolver interface {
	ResolveCouponForRender(ctx context.Context, tenantID, id pgtype.UUID) (*CouponRenderResult, error)
}

// AffiliateLinkResolver resolves an affiliate provider ID + original URL to
// the tracking-injected URL. Implemented by coupons.Service. The tenantID
// scopes the lookup (BE-LOW-001).
type AffiliateLinkResolver interface {
	ResolveAffiliateLinkForRender(ctx context.Context, tenantID, providerID pgtype.UUID, originalURL string) (*AffiliateLinkRenderResult, error)
}

// CouponRenderResult is the resolved coupon data for render-time injection.
// Mirrors coupons.RenderedCoupon but kept here to avoid an import cycle.
type CouponRenderResult struct {
	ID            string
	Title         string
	Code          string
	DiscountType  *string
	DiscountValue *string
	ProviderName  *string
	ExpiresAt     *string
	IsExpired     bool
	ReferralURL   *string
	Status        string
}

// AffiliateLinkRenderResult is the resolved affiliate link with tracking params.
type AffiliateLinkRenderResult struct {
	ResolvedURL string
}

// resolveCouponEmbedsAndAffiliateLinks post-processes the rendered HTML to
// resolve coupon_embed placeholders and rewrite affiliate link hrefs (§55 P4).
// The tenantID scopes all resolver lookups so a render request cannot resolve
// a coupon or provider from another tenant (BE-LOW-001). Best-effort:
// resolution failures are logged and the original placeholder/link is left
// in place.
func resolveCouponEmbedsAndAffiliateLinks(ctx context.Context, tenantID pgtype.UUID, input string, couponResolver CouponResolver, affiliateResolver AffiliateLinkResolver) string {
	if couponResolver == nil && affiliateResolver == nil {
		return input
	}

	doc, err := html.Parse(bytes.NewReader([]byte(input)))
	if err != nil {
		slog.Warn("renderer: failed to parse HTML for coupon/affiliate resolution", "error", err)
		return input
	}

	var resolve func(*html.Node)
	resolve = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			if couponResolver != nil && getAttr(n, "data-type") == "coupon-embed" {
				couponIDStr := getAttr(n, "data-coupon-id")
				if couponIDStr != "" {
					resolveCouponEmbed(ctx, tenantID, n, couponIDStr, couponResolver)
				}
			}
		}

		if n.Type == html.ElementNode && n.Data == "a" && affiliateResolver != nil {
			href := getAttr(n, "href")
			if href != "" && isExternalLink(href) {
				resolveAffiliateLink(ctx, tenantID, n, href, affiliateResolver)
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			resolve(c)
		}
	}
	resolve(doc)

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		slog.Warn("renderer: failed to render resolved HTML", "error", err)
		return input
	}

	return buf.String()
}

// resolveCouponEmbed replaces a coupon-embed placeholder div with the resolved
// coupon widget HTML. If the coupon is expired or not found, an "Offer expired"
// message is rendered instead. The display_mode attribute ("compact" or "full")
// controls the widget variant (Section 55). The tenantID scopes the coupon
// lookup (BE-LOW-001).
func resolveCouponEmbed(ctx context.Context, tenantID pgtype.UUID, n *html.Node, couponIDStr string, resolver CouponResolver) {
	var couponID pgtype.UUID
	if err := couponID.Scan(couponIDStr); err != nil {
		slog.Warn("renderer: invalid coupon_id in embed", "coupon_id", couponIDStr, "error", err)
		return
	}

	coupon, err := resolver.ResolveCouponForRender(ctx, tenantID, couponID)
	if err != nil {
		slog.Warn("renderer: failed to resolve coupon embed", "coupon_id", couponIDStr, "error", err)
		return
	}

	displayMode := getAttr(n, "data-display-mode")

	// Build the replacement widget HTML.
	widgetHTML := buildCouponWidgetHTML(coupon, displayMode)

	// Parse the widget HTML and replace the placeholder's children.
	widgetDoc, err := html.ParseFragment(strings.NewReader(widgetHTML), n.Parent)
	if err != nil {
		slog.Warn("renderer: failed to parse coupon widget HTML", "error", err)
		return
	}

	// Clear existing children.
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		n.RemoveChild(c)
		c = next
	}
	// Remove data attributes (no longer needed after resolution).
	n.Attr = filterAttrs(n.Attr, func(a html.Attribute) bool {
		return a.Key != "data-type" && a.Key != "data-coupon-id" && a.Key != "data-display-mode"
	})
	// Add a class for styling.
	n.Attr = append(n.Attr, html.Attribute{Key: "class", Val: "coupon-embed-widget"})

	// Append widget nodes.
	for _, w := range widgetDoc {
		n.AppendChild(w)
	}
}

// resolveAffiliateLink rewrites an <a> href to inject affiliate tracking params
// (Section 55 P4). The TipTap link node stores affiliate_provider_id as a
// data-affiliate-provider-id attribute on the <a> tag (emitted by the SSR
// script). If the attribute is present and the resolver returns a tracking-
// injected URL, the href is replaced. Links without the attribute or whose
// resolution fails are left unchanged (best-effort). The tenantID scopes the
// provider lookup (BE-LOW-001).
func resolveAffiliateLink(ctx context.Context, tenantID pgtype.UUID, n *html.Node, href string, resolver AffiliateLinkResolver) {
	providerIDStr := getAttr(n, "data-affiliate-provider-id")
	if providerIDStr == "" {
		return
	}

	var providerID pgtype.UUID
	if err := providerID.Scan(providerIDStr); err != nil {
		slog.Warn("renderer: invalid affiliate_provider_id on link", "provider_id", providerIDStr, "error", err)
		return
	}

	result, err := resolver.ResolveAffiliateLinkForRender(ctx, tenantID, providerID, href)
	if err != nil {
		slog.Warn("renderer: failed to resolve affiliate link", "provider_id", providerIDStr, "href", href, "error", err)
		return
	}

	if result.ResolvedURL == "" || result.ResolvedURL == href {
		return
	}

	// Replace the href attribute with the tracking-injected URL.
	for i, a := range n.Attr {
		if a.Key == "href" {
			n.Attr[i].Val = result.ResolvedURL
			return
		}
	}
}

// buildCouponWidgetHTML builds the HTML for a resolved coupon widget.
// displayMode "compact" renders a minimal one-line widget; "full" (default)
// renders the complete card. The referral_url is scheme-validated (http/https
// only) before injection to prevent stored XSS via javascript: URIs (§5.1).
func buildCouponWidgetHTML(c *CouponRenderResult, displayMode string) string {
	if c.IsExpired || c.Status != "active" {
		return `<div class="coupon-expired">This offer has expired.</div>`
	}

	compact := displayMode == "compact"

	var b strings.Builder

	b.WriteString(`<div class="coupon-card`)

	if compact {
		b.WriteString(` coupon-card-compact`)
	}

	b.WriteString(`">`)

	// Title
	b.WriteString(fmt.Sprintf(`<div class="coupon-title">%s</div>`, htmlpkg.EscapeString(c.Title)))

	if compact {
		// Compact mode: code + CTA only, no discount/provider/expiry details.
		b.WriteString(fmt.Sprintf(`<div class="coupon-code"><code>%s</code></div>`, htmlpkg.EscapeString(c.Code)))
	} else {
		// Code
		b.WriteString(fmt.Sprintf(`<div class="coupon-code"><code>%s</code></div>`, htmlpkg.EscapeString(c.Code)))

		// Discount
		if c.DiscountType != nil && c.DiscountValue != nil {
			dt := *c.DiscountType
			dv := *c.DiscountValue

			switch dt {
			case "percent":
				b.WriteString(fmt.Sprintf(`<div class="coupon-discount">%s%% off</div>`, htmlpkg.EscapeString(dv)))
			case "free_shipping":
				b.WriteString(`<div class="coupon-discount">Free Shipping</div>`)
			default:
				b.WriteString(fmt.Sprintf(`<div class="coupon-discount">%s off</div>`, htmlpkg.EscapeString(dv)))
			}
		}

		// Provider
		if c.ProviderName != nil {
			b.WriteString(fmt.Sprintf(`<div class="coupon-provider">%s</div>`, htmlpkg.EscapeString(*c.ProviderName)))
		}

		// Expiry
		if c.ExpiresAt != nil {
			b.WriteString(fmt.Sprintf(`<div class="coupon-expires">Expires: %s</div>`, htmlpkg.EscapeString(*c.ExpiresAt)))
		}
	}

	// CTA — validate the referral_url scheme before injecting into href to
	// prevent stored XSS via javascript: URIs (§5.1). The widget HTML is
	// built AFTER SanitizeHTML runs, so this is the only scheme check.
	if c.ReferralURL != nil && isSafeURL(*c.ReferralURL) {
		b.WriteString(fmt.Sprintf(`<a href="%s" rel="nofollow noopener noreferrer" target="_blank" class="coupon-cta">Get this deal</a>`, htmlpkg.EscapeString(*c.ReferralURL)))
	}

	b.WriteString(`</div>`)

	return b.String()
}

// getAttr returns the value of the named attribute, or "" if not present.
func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}

	return ""
}

// filterAttrs returns only attributes that pass the predicate.
func filterAttrs(attrs []html.Attribute, keep func(html.Attribute) bool) []html.Attribute {
	result := attrs[:0]

	for _, a := range attrs {
		if keep(a) {
			result = append(result, a)
		}
	}

	return result
}

// isExternalLink returns true if the href is an absolute http/https URL.
func isExternalLink(href string) bool {
	u, err := url.Parse(href)
	if err != nil {
		return false
	}

	return u.Scheme == "http" || u.Scheme == "https"
}

// isSafeURL returns true if the URL uses http, https, or is a relative path.
// Rejects javascript:, data:, vbscript: and other dangerous schemes. Used to
// validate referral_url before injecting into <a href> in the coupon widget
// (§5.1 — the widget HTML is built after SanitizeHTML, so this is the only
// scheme check).
func isSafeURL(raw string) bool {
	lower := strings.TrimSpace(strings.ToLower(raw))
	if lower == "" || strings.HasPrefix(lower, "/") || strings.HasPrefix(lower, "#") || strings.HasPrefix(lower, "?") {
		return true
	}

	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

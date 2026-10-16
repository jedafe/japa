package literals

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jedafe/japa/internal/geoip"
	"github.com/jedafe/japa/internal/literals/countrydata"
	"github.com/jedafe/japa/internal/visitor"
)

// platformNameResolver is the process-wide provider for the platform/tenant
// name used by the Platform_Name and Tenant_Name tokens. Wired once during
// server bootstrap via SetPlatformNameResolver (e.g. to the site.name
// setting); unset means the tokens resolve empty unless a caller passes an
// explicit name.
var platformNameResolver atomic.Value // stores func(context.Context) string

// SetPlatformNameResolver registers the function used to resolve the
// platform name for literal rendering when a caller does not supply one
// explicitly. Call once during server bootstrap.
func SetPlatformNameResolver(fn func(ctx context.Context) string) {
	platformNameResolver.Store(fn)
}

// resolvePlatformName returns explicit when non-empty, otherwise delegates
// to the registered resolver.
func resolvePlatformName(ctx context.Context, explicit string) string {
	if explicit != "" {
		return explicit
	}

	if fn, ok := platformNameResolver.Load().(func(context.Context) string); ok && fn != nil {
		return fn(ctx)
	}

	return ""
}

// NewRequestContext builds a RenderContext for the current HTTP request:
// UTC now, Accept-Language primary tag, platform name/id, and the geo fields
// filled by FillRequestContext. Use this in any handler that renders
// {{...}} placeholders so all surfaces resolve identically.
func NewRequestContext(r *http.Request, platformName string) RenderContext {
	rc := RenderContext{
		Now:          time.Now().UTC(),
		Locale:       requestLocale(r),
		PlatformName: resolvePlatformName(r.Context(), platformName),
		raw:          IsRawRequest(r),
	}
	FillRequestContext(r, &rc)

	return rc
}

// NewContext builds a RenderContext without an *http.Request — for non-HTTP
// renderers (widget rendering, RSS worker tasks, digest/email generation).
// Geo fields are read from any VisitorContext/GeoIP values already attached
// to ctx; locale stays empty. platformName resolves the same way as
// NewRequestContext.
func NewContext(ctx context.Context, platformName string) RenderContext {
	rc := RenderContext{
		Now:          time.Now().UTC(),
		PlatformName: resolvePlatformName(ctx, platformName),
	}

	FillContext(ctx, &rc)

	return rc
}

// requestLocale extracts the primary language tag from Accept-Language
// ("en-US,en;q=0.9" -> "en"). Full i18n middleware integration is deferred.
func requestLocale(r *http.Request) string {
	accept := r.Header.Get("Accept-Language")
	if accept == "" {
		return "en"
	}

	primary := strings.Split(accept, ",")[0]
	primary = strings.TrimSpace(primary)
	primary = strings.Split(primary, "-")[0]
	primary = strings.ToLower(primary)

	if primary == "" {
		return "en"
	}

	return primary
}

// FillRequestContext populates the geo fields of a RenderContext from the
// request's middleware context (ADR-064). See FillContext for the field
// semantics.
func FillRequestContext(r *http.Request, rc *RenderContext) {
	FillContext(r.Context(), rc)
}

// FillContext populates the geo fields of a RenderContext from a context
// carrying middleware values (ADR-064):
//
//   - Source_Code / Source_Country — the visitor's origin. The VisitorContext
//     OriginCountry wins (profile origin_country, then jc_journey_token for
//     guests); GeoIP is the fallback. The GeoIP-resolved country name is used
//     when the codes match, otherwise the countrydata lookup supplies it.
//   - Destination_Code / Destination_Country — the visitor's declared
//     relocation destination (profile destination_country custom field, or
//     the guest journey cookie/query). Codes are ISO-2; the name resolves via
//     the countrydata lookup.
//
// Both fields stay empty when no signal exists — the literal's own fallback
// value (or an inline {{Key|fallback}}) then applies.
func FillContext(ctx context.Context, rc *RenderContext) {
	geo := geoip.FromContext(ctx)
	vc := visitor.FromContext(ctx)

	srcCode := vc.OriginCountry
	if srcCode == "" {
		srcCode = geo.CountryCode
	}

	rc.SourceCode = srcCode

	if srcCode != "" {
		if srcCode == geo.CountryCode && geo.IsValid {
			rc.SourceCountry = geo.CountryName
		} else {
			rc.SourceCountry = countrydata.NameForCode(srcCode)
		}
	}

	if vc.DestinationCountry != "" {
		rc.DestinationCode = vc.DestinationCountry
		rc.DestinationCountry = countrydata.NameForCode(vc.DestinationCountry)
	}
}

// IsRawRequest reports whether the caller asked for unresolved template
// source. Editor flows append ?raw=1 when fetching an entity so the author's
// original {{...}} syntax is preserved for editing — without it, the resolved
// value would be persisted on the next save and the template lost.
func IsRawRequest(r *http.Request) bool {
	return r.URL.Query().Has("raw")
}

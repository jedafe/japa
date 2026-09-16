package literals

import (
	"net/http"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/geoip"
	"github.com/jedafe/japa/internal/literals/countrydata"
	"github.com/jedafe/japa/internal/visitor"
)

// NewRequestContext builds a RenderContext for the current HTTP request:
// UTC now, Accept-Language primary tag, platform name/id, and the geo fields
// filled by FillRequestContext. Use this in any handler that renders
// {{...}} placeholders so all surfaces resolve identically.
func NewRequestContext(r *http.Request, platformName string) RenderContext {
	rc := RenderContext{
		Now:        time.Now().UTC(),
		Locale:     requestLocale(r),
		PlatformName: platformName,
	}
	FillRequestContext(r, &rc)

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
// request's middleware context (ADR-064):
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
func FillRequestContext(r *http.Request, rc *RenderContext) {
	geo := geoip.FromContext(r.Context())
	vc := visitor.FromContext(r.Context())

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

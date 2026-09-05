// Package universities — see sections/39-space-types-university.md for the
// university custom-field model and acronym/initials rules.
// Ref: japaconnect-engineering/sections/39-space-types-university.md
package universities

import (
	"net/url"
	"regexp"
	"strings"
)

// nameInitialsOverride maps normalized university names to their commonly
// used short codes / acronyms. It covers stylized cases that cannot be
// derived mechanically from a domain, such as "UoPeople" or "UNILAG".
// Ref: japaconnect-engineering/sections/39-space-types-university.md.
var nameInitialsOverride = map[string]string{
	"university of lagos":                              "UNILAG",
	"university of ibadan":                             "UI",
	"university of nigeria nsukka":                     "UNN",
	"obafemi awolowo university":                       "OAU",
	"obafemi awolowo university ile ife":               "OAU",
	"university of benin":                              "UNIBEN",
	"university of port harcourt":                      "UNIPORT",
	"university of ilorin":                             "UNILORIN",
	"university of jos":                                "UNIJOS",
	"university of maiduguri":                          "UNIMAID",
	"university of abuja":                              "UNIABUJA",
	"university of uyo":                                "UNIUYO",
	"university of calabar":                            "UNICAL",
	"delta state university":                           "DELSU",
	"edo state university":                             "EDSU",
	"rivers state university":                          "RSU",
	"imo state university":                             "IMSU",
	"anambra state university":                         "ANSU",
	"nasarawa state university":                        "NSUK",
	"kogi state university":                            "KSU",
	"taraba state university":                          "TASU",
	"sokoto state university":                          "SSU",
	"yobe state university":                            "YSU",
	"kwara state university":                           "KWASU",
	"ogun state university":                            "OSU",
	"osun state university":                            "UNIOSUN",
	"ondo state university":                            "OOU",
	"ekiti state university":                           "EKSU",
	"enugu state university of science and technology": "ESUT",
	"abia state university":                            "ABSU",
	"akwa ibom state university":                       "AKSU",
	"bayero university kano":                           "BUK",
	"usmanu danfodiyo university":                      "UDUSOK",
	"modibbo adama university of technology":           "MAUTECH",
	"federal university of technology minna":           "FUTMINNA",
	"federal university of technology akure":           "FUTA",
	"federal university of technology owerri":          "FUTO",
	"federal university of agriculture abeokuta":       "FUNAAB",
	"federal university of agriculture makurdi":        "FUAM",
	"university of agriculture benin":                  "UNIABEN",
	"nnamdi azikiwe university":                        "UNIZIK",
	"covenant university":                              "CU",
	"babcock university":                               "BU",
	"redeemers university":                             "RUN",
	"american university of nigeria":                   "AUN",
	"pan atlantic university":                          "PAU",
	"university of the people":                         "UoPeople",
	"university of the people tuition free":            "UoPeople",
	"massachusetts institute of technology":            "MIT",
	"california institute of technology":               "Caltech",
	"university of california berkeley":                "UCB",
	"university of california los angeles":             "UCLA",
	"university of southern california":                "USC",
	"university of texas at austin":                    "UT Austin",
	"university of oxford":                             "Oxford",
	"university of cambridge":                          "Cambridge",
	"imperial college london":                          "Imperial",
	"london school of economics":                       "LSE",
	"university of edinburgh":                          "Edinburgh",
	"university of manchester":                         "Manchester",
	"university of bristol":                            "Bristol",
	"university of toronto":                            "UofT",
	"university of british columbia":                   "UBC",
	"mcgill university":                                "McGill",
	"university of alberta":                            "UAlberta",
	"university of waterloo":                           "Waterloo",
}

// domainInitialsOverride maps the first label of a domain to its preferred
// short code when simple upper-casing would be wrong (e.g. "uopeople" →
// "UoPeople" and "caltech" → "Caltech").
var domainInitialsOverride = map[string]string{
	"uopeople":  "UoPeople",
	"caltech":   "Caltech",
	"harvard":   "Harvard",
	"stanford":  "Stanford",
	"oxford":    "Oxford",
	"cambridge": "Cambridge",
	"imperial":  "Imperial",
	"yale":      "Yale",
	"princeton": "Princeton",
	"columbia":  "Columbia",
	"cornell":   "Cornell",
	"dartmouth": "Dartmouth",
	"duke":      "Duke",
	"brown":     "Brown",
	"chicago":   "Chicago",
	"northwest": "Northwestern",
	"oauife":    "OAU",
	"oau":       "OAU",
}

var (
	stopWords = map[string]bool{
		"of": true, "the": true, "and": true, "&": true, "in": true,
		"at": true, "for": true, "de": true, "di": true, "del": true,
		"della": true, "la": true, "le": true, "et": true, "a": true,
	}
	multipleSpaces = regexp.MustCompile(`\s+`)
	nonAlpha       = regexp.MustCompile(`[^a-z0-9\s]+`)
)

// DeriveInitials returns the best short code / acronym for a university based
// on its canonical name, domain list, and primary website. It uses a curated
// override map for well-known acronyms, falls back to the primary domain label,
// and finally derives first-letters from the name when no domain is available.
// The result is intended as a default; Wikidata P1813 and admin edits can refine
// it later.
func DeriveInitials(name string, domains, webPages []string) string {
	key := normalizeName(name)
	if v, ok := nameInitialsOverride[key]; ok {
		return v
	}

	host := ""

	for _, d := range domains {
		if strings.TrimSpace(d) != "" {
			host = strings.TrimSpace(d)
			break
		}
	}

	if host == "" && len(webPages) > 0 {
		if u, err := url.Parse(strings.TrimSpace(webPages[0])); err == nil && u.Host != "" {
			host = u.Host
		}
	}

	if host != "" {
		initial := initialsFromHost(host)
		if initial != "" {
			return initial
		}
	}

	return fallbackInitialsFromName(name)
}

// normalizeName strips punctuation and lower-cases a university name for lookup
// in the override map.
func normalizeName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonAlpha.ReplaceAllString(s, " ")
	s = multipleSpaces.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)

	return s
}

// initialsFromHost extracts the leading label from a host, drops common
// subdomains, applies the domain override map, and upper-cases the remainder.
func initialsFromHost(host string) string {
	host = strings.ToLower(host)
	if strings.HasPrefix(host, "www.") || strings.HasPrefix(host, "m.") {
		host = strings.SplitN(host, ".", 2)[1]
	}

	parts := strings.Split(host, ".")
	if len(parts) == 0 {
		return ""
	}
	// Drop common leading subdomains (www, m, web, en, etc.) when a real domain
	// follows.
	for len(parts) > 2 && isSubdomain(parts[0]) {
		parts = parts[1:]
	}

	first := parts[0]
	if v, ok := domainInitialsOverride[first]; ok {
		return v
	}

	return strings.ToUpper(first)
}

func isSubdomain(s string) bool {
	switch s {
	case "www", "m", "web", "en", "www2", "old", "portal", "mail", "library":
		return true
	}

	return false
}

// fallbackInitialsFromName builds an acronym from the first letter of each
// significant word in the name (excluding common stop words). It is a last
// resort when no domain is available.
func fallbackInitialsFromName(name string) string {
	s := normalizeName(name)
	words := strings.Split(s, " ")

	var b strings.Builder

	for _, w := range words {
		if w == "" || stopWords[w] || len(w) <= 1 {
			continue
		}

		b.WriteByte(w[0])
	}

	if b.Len() == 0 {
		return ""
	}

	return strings.ToUpper(b.String())
}

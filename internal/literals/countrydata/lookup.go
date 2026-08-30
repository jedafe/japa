package countrydata

import "strings"

// AdjectiveFor returns the adjective for a country code or country name.
// It is case-insensitive and falls back to the input if no match is found.
func AdjectiveFor(input string) string {
	if c, ok := ByCode[strings.ToUpper(strings.TrimSpace(input))]; ok {
		return c.Adjective
	}
	if c, ok := ByName[strings.ToLower(strings.TrimSpace(input))]; ok {
		return c.Adjective
	}
	return ""
}

// CodeForName returns the ISO-2 code for a country name.
func CodeForName(input string) string {
	if c, ok := ByName[strings.ToLower(strings.TrimSpace(input))]; ok {
		return c.Code
	}
	return ""
}

// NameForCode returns the country name for an ISO-2 code.
func NameForCode(code string) string {
	if c, ok := ByCode[strings.ToUpper(strings.TrimSpace(code))]; ok {
		return c.Name
	}
	return ""
}

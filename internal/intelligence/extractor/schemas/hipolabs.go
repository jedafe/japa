// Deterministic Hipolabs (universities.hipolabs.com) mapper: converts the
// API's JSON array straight into university extraction triples — no LLM
// call, no token cost. Selected per-source via provider_config
// {"deterministic": "hipolabs"}; the extract worker checks it before the
// LLM dispatch. Records still flow dedup → promote → UniversityPromoter.
// Lives in schemas (a leaf package) so the worker can reach it — importing
// the extractor package would cycle worker → extractor → ai → auth → worker.
package schemas

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DeterministicExtractorHipolabs is the provider_config "deterministic"
// value selecting this mapper.
const DeterministicExtractorHipolabs = "hipolabs"

// hipolabsRecord mirrors the Hipolabs API row shape.
type hipolabsRecord struct {
	Name          string   `json:"name"`
	AlphaTwoCode  string   `json:"alpha_two_code"`
	Country       string   `json:"country"`
	StateProvince string   `json:"state-province"`
	Domains       []string `json:"domains"`
	WebPages      []string `json:"web_pages"`
}

// HipolabsExtraction is one deterministic extraction result — marshalled
// field-triples ready for the extracted column + its dedup key.
type HipolabsExtraction struct {
	Extracted   []byte
	ExternalKey string
}

// ExtractHipolabs maps a Hipolabs response body to extraction triples in
// the {"value","confidence","evidence"} shape the promoter parses. The
// ExternalKey (primary domain, else slugified name) is the dedup key that
// makes re-crawls converge on existing records.
func ExtractHipolabs(body string) ([]HipolabsExtraction, error) {
	var rows []hipolabsRecord
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		return nil, fmt.Errorf("hipolabs: parse body: %w", err)
	}

	triple := func(v any, ev string) map[string]any {
		t := map[string]any{"value": v, "confidence": 1.0}
		if ev != "" {
			t["evidence"] = ev
		}

		return t
	}

	out := make([]HipolabsExtraction, 0, len(rows))
	for _, r := range rows {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			continue
		}

		ex := map[string]any{"name": triple(name, name)}
		if code := strings.ToUpper(strings.TrimSpace(r.AlphaTwoCode)); code != "" {
			// alpha-2 is fine: the promoter's CountryCodeAlpha2 accepts both
			ex["country"] = triple([]string{code}, r.Country)
		}
		if len(r.WebPages) > 0 {
			if u := strings.TrimSpace(r.WebPages[0]); u != "" {
				ex["official_website_url"] = triple(u, u)
				ex["source_url"] = triple(u, u)
			}
		}

		key := ""
		if len(r.Domains) > 0 {
			key = strings.ToLower(strings.TrimSpace(r.Domains[0]))
		}
		if key == "" {
			key = strings.ToLower(strings.ReplaceAll(name, " ", "-"))
		}

		raw, err := json.Marshal(ex)
		if err != nil {
			continue
		}

		out = append(out, HipolabsExtraction{Extracted: raw, ExternalKey: key})
	}

	return out, nil
}

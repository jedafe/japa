package curator

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// Tabular curates tabular crawl-method channels: the raw doc body is the
// CSV/TSV bytes and every data row becomes one record. The channel's
// provider_config drives the mapping:
//
//	{
//	  "column_map":   {"Course": "title", "Fee": "value", "Currency": "unit"},
//	  "key_columns":  ["Course"],              // external_key hash inputs
//	  "title_column": "Course",                // fallback when unmapped
//	  "entity_type":  "dataset"                // record entity-type override
//	}
//
// Mapped columns land on the dataset schema fields; unmapped columns are
// preserved verbatim under the `data` jsonb field so nothing is lost.
// XLSX bodies are not supported (no spreadsheet dependency in the module) —
// the channel should publish CSV or the record falls back to the extract
// pipeline.
type Tabular struct{}

// Handles returns the crawl method this curator serves.
func (Tabular) Handles() string { return string(sources.CrawlMethodTabular) }

// tabularConfig is the channel provider_config contract for tabular curation.
type tabularConfig struct {
	// ColumnMap maps a CSV header name to a dataset schema field name.
	ColumnMap map[string]string `json:"column_map"`
	// KeyColumns are the CSV headers whose values hash into the external
	// key. Empty hashes the whole row.
	KeyColumns []string `json:"key_columns"`
	// TitleColumn names the CSV column used for the required `title` field
	// when column_map does not already map one.
	TitleColumn string `json:"title_column"`
	// EntityType overrides the record entity type (default: channel's
	// entity_types[0], i.e. dataset).
	EntityType string `json:"entity_type"`
	// Delimiter overrides the field delimiter ('\t' for TSV, ';' for
	// European CSVs). Default: ','.
	Delimiter string `json:"delimiter"`
	// HasHeader reports whether the first row is a header row (default true).
	// When false, columns are addressed positionally as "col0", "col1", ….
	HasHeader *bool `json:"has_header"`
}

// Curate parses the tabular body and returns one record per data row.
func (Tabular) Curate(_ context.Context, body []byte, contentType string, ch sources.Channel) ([]Record, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, errors.New("curator: tabular: empty body")
	}

	// XLSX/ODS are binary zip containers — without a spreadsheet dependency
	// they cannot be parsed here; signal clearly so the channel config is
	// corrected (publish CSV) rather than silently producing garbage.
	if strings.Contains(contentType, "spreadsheet") || strings.Contains(contentType, "excel") ||
		len(body) > 2 && body[0] == 'P' && body[1] == 'K' {
		return nil, errors.New("curator: tabular: binary spreadsheet bodies are not supported — publish CSV/TSV")
	}

	cfg := parseTabularConfig(ch.ProviderConfig)

	headers, rows, err := readTabular(body, contentType, cfg)
	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return nil, errors.New("curator: tabular: no data rows")
	}

	entityType := resolveEntityType(cfg.EntityType, ch)
	out := make([]Record, 0, len(rows))

	for _, row := range rows {
		ext := tabularRowToExtraction(headers, row, cfg, ch)
		out = append(out, Record{
			Extraction:  validate(ext, entityType),
			ExternalKey: tabularRowKey(headers, row, cfg),
			EntityType:  entityType,
		})
	}

	return out, nil
}

// parseTabularConfig decodes the channel's provider_config into the tabular
// mapping contract. Malformed or absent config yields zero values — the
// positional defaults still produce a record per row.
func parseTabularConfig(raw []byte) tabularConfig {
	var cfg tabularConfig
	if len(raw) == 0 {
		return cfg
	}
	// Tolerate a malformed document: defaults still emit row records.
	_ = json.Unmarshal(raw, &cfg)

	return cfg
}

// readTabular parses the CSV/TSV body into headers + data rows.
func readTabular(body []byte, contentType string, cfg tabularConfig) ([]string, [][]string, error) {
	r := csv.NewReader(strings.NewReader(string(body)))
	r.FieldsPerRecord = -1 // ragged rows are tolerated; short rows pad empty
	r.TrimLeadingSpace = true

	switch {
	case cfg.Delimiter != "":
		r.Comma = rune(cfg.Delimiter[0])
	case strings.Contains(contentType, "tab-separated"):
		r.Comma = '\t'
	}

	var rows [][]string

	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, nil, fmt.Errorf("curator: tabular: parse row %d: %w", len(rows), err)
		}
		// Skip fully blank rows.
		blank := true

		for _, cell := range rec {
			if strings.TrimSpace(cell) != "" {
				blank = false
				break
			}
		}

		if !blank {
			rows = append(rows, rec)
		}
	}

	if len(rows) == 0 {
		return nil, nil, errors.New("curator: tabular: no rows")
	}

	hasHeader := cfg.HasHeader == nil || *cfg.HasHeader

	var headers []string
	if hasHeader {
		headers = rows[0]
		rows = rows[1:]
	} else {
		headers = make([]string, 0, len(rows[0]))
		for i := range rows[0] {
			headers = append(headers, fmt.Sprintf("col%d", i))
		}
	}

	for i := range headers {
		headers[i] = strings.TrimSpace(headers[i])
	}

	return headers, rows, nil
}

// tabularRowToExtraction maps one row onto the dataset schema: mapped
// columns land on their field, unmapped columns collect under `data`, and
// `title` falls back to title_column or the first column.
func tabularRowToExtraction(headers, row []string, cfg tabularConfig, ch sources.Channel) extractor.Extraction {
	ext := make(extractor.Extraction)

	data := map[string]any{}
	rowJSON := tabularRowEvidence(headers, row)

	for i, cell := range row {
		if i >= len(headers) {
			break
		}

		col := headers[i]

		value := strings.TrimSpace(cell)
		if value == "" {
			continue
		}

		if name, mapped := cfg.ColumnMap[col]; mapped && name != "" {
			field(ext, name, tabularCoerceValue(name, value), value)
			continue
		}

		data[col] = value
	}

	// title fallback: title_column, then the first populated column.
	if _, ok := ext["title"]; !ok {
		idx := indexOfHeader(headers, cfg.TitleColumn)
		if idx < 0 {
			idx = 0
		}

		if idx < len(row) && strings.TrimSpace(row[idx]) != "" {
			field(ext, "title", strings.TrimSpace(row[idx]), row[idx])
		}
	}

	if len(data) > 0 {
		field(ext, "data", data, rowJSON)
	}

	if ch.EntryUrl != "" {
		field(ext, "source_url", ch.EntryUrl, ch.EntryUrl)
	}

	return ext
}

// tabularCoerceValue shapes a mapped cell for its schema field type —
// country fields take a single-element array, everything else stays a
// string (the numeric coercer accepts numeric strings).
func tabularCoerceValue(fieldName, value string) any {
	if fieldName == "country" {
		return []string{value}
	}

	return value
}

// tabularRowKey derives the row's external key: a hash over the configured
// key_columns values (in header order), or over the full row when none are
// configured. Stable across re-crawls of an unchanged document.
func tabularRowKey(headers, row []string, cfg tabularConfig) string {
	var parts []string

	if len(cfg.KeyColumns) > 0 {
		for _, kc := range cfg.KeyColumns {
			idx := indexOfHeader(headers, kc)
			if idx >= 0 && idx < len(row) {
				parts = append(parts, strings.TrimSpace(row[idx]))
			}
		}
	}

	if len(parts) == 0 {
		for _, cell := range row {
			parts = append(parts, strings.TrimSpace(cell))
		}
	}

	return "row:" + hashKey(parts...)
}

// tabularRowEvidence renders the row as a compact JSON object for the
// shared `data` field's evidence string.
func tabularRowEvidence(headers, row []string) string {
	m := make(map[string]string, len(headers))

	for i, h := range headers {
		if i < len(row) {
			m[h] = row[i]
		}
	}

	b, err := json.Marshal(m)
	if err != nil {
		return strings.Join(row, ",")
	}

	return string(b)
}

func indexOfHeader(headers []string, name string) int {
	for i, h := range headers {
		if h == name {
			return i
		}
	}

	return -1
}

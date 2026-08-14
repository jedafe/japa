// Package promoter — shared extraction field accessors and comparison
// helpers used by all domain promoters. Extracted from scholarship.go so
// each domain promoter (job, visa, housing, remittance, cost_of_living,
// and the 7 guide content types) can reuse them without duplication.
package promoter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
)

// PersonaResolver is the minimal interface a promoter store needs to resolve
// an AI persona's backing user_id. All promoter Store interfaces embed this
// so the createPath can resolve the source's persona → author_id.
type PersonaResolver interface {
	GetPersonaUserID(ctx context.Context, tenantID, personaID pgtype.UUID) (pgtype.UUID, error)
}

// resolveAuthorID resolves the author_id for a promoted content row. If the
// source has a persona_id, it looks up the persona's backing user_id and
// uses that as the author. If the persona lookup fails or the source has no
// persona, it falls back to the promoter's default authorID. This fixes the
// FK violation where defaultTenantID was used as contents.author_id.
func resolveAuthorID(ctx context.Context, resolver PersonaResolver, source gen.IntelSource, tenantID, fallbackAuthorID pgtype.UUID) pgtype.UUID {
	if !source.PersonaID.Valid {
		return fallbackAuthorID
	}
	userID, err := resolver.GetPersonaUserID(ctx, tenantID, source.PersonaID)
	if err != nil || !userID.Valid {
		slog.Warn("promoter: persona user_id lookup failed, using fallback author",
			"persona_id", source.PersonaID.String(),
			"error", err)
		return fallbackAuthorID
	}
	return userID
}

// parseExtractionShared decodes the raw extracted JSON blob into an
// extractor.Extraction map. Shared across all domain promoters.
// Handles both the current format ({field: {value, confidence, evidence}})
// and the legacy format ({field: "plain string value"}) by wrapping bare
// values into FieldExtraction{Value: v}.
func parseExtractionShared(raw []byte) (extractor.Extraction, error) {
	if len(raw) == 0 {
		return extractor.Extraction{}, nil
	}
	var ext extractor.Extraction
	if err := json.Unmarshal(raw, &ext); err == nil {
		return ext, nil
	}
	// Fallback: try legacy format where values are plain strings/scalars
	// instead of {value, confidence, evidence} objects.
	var legacy map[string]json.RawMessage
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return nil, fmt.Errorf("parse extraction: %w", err)
	}
	ext = make(extractor.Extraction, len(legacy))
	for k, rawVal := range legacy {
		var fe extractor.FieldExtraction
		if err := json.Unmarshal(rawVal, &fe); err == nil && fe.Value != nil {
			ext[k] = fe
			continue
		}
		// Bare scalar — wrap it.
		var scalar any
		if err := json.Unmarshal(rawVal, &scalar); err != nil {
			continue
		}
		ext[k] = extractor.FieldExtraction{Value: scalar, Confidence: 0.5}
	}
	return ext, nil
}

// stringFieldShared returns the string form of an extraction field, or "".
func stringFieldShared(ext extractor.Extraction, key string) string {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return ""
	}
	switch v := f.Value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// firstCountryShared returns the first element of a country_array field.
func firstCountryShared(ext extractor.Extraction, key string) string {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return ""
	}
	if arr, ok := f.Value.([]any); ok && len(arr) > 0 {
		if s, ok := arr[0].(string); ok {
			return strings.TrimSpace(s)
		}
	}
	if s, ok := f.Value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func stringPtrFieldShared(ext extractor.Extraction, key string) *string {
	s := stringFieldShared(ext, key)
	if s == "" {
		return nil
	}
	return &s
}

func stringSliceFieldShared(ext extractor.Extraction, key string) []string {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return nil
	}
	arr, ok := f.Value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			if t := strings.TrimSpace(s); t != "" {
				out = append(out, t)
			}
		}
	}
	if len(out) == 0 {
		return []string{}
	}
	return out
}

func dateFieldShared(ext extractor.Extraction, key string) pgtype.Date {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return pgtype.Date{}
	}
	s, ok := f.Value.(string)
	if !ok {
		return pgtype.Date{}
	}
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	if err != nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: t, Valid: true}
}

func numericFieldShared(ext extractor.Extraction, key string) pgtype.Numeric {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return pgtype.Numeric{}
	}
	s := ""
	switch v := f.Value.(type) {
	case string:
		s = strings.TrimSpace(v)
	case float64:
		s = fmt.Sprintf("%g", v)
	default:
		s = fmt.Sprintf("%v", v)
	}
	if s == "" {
		return pgtype.Numeric{}
	}
	rat, ok := new(big.Rat).SetString(s)
	if !ok {
		return pgtype.Numeric{}
	}
	num := new(big.Int).Set(rat.Num())
	den := new(big.Int).Set(rat.Denom())
	exp := int32(0)
	ten := big.NewInt(10)
	for den.Cmp(big.NewInt(1)) > 0 {
		if new(big.Int).Mod(den, ten).Sign() != 0 {
			num.Mul(num, ten)
			exp--
			den.Quo(den, ten)
			continue
		}
		num.Mul(num, ten)
		exp--
		den.Quo(den, ten)
	}
	return pgtype.Numeric{Int: num, Exp: exp, Valid: true}
}

func jsonbFieldShared(ext extractor.Extraction, key string) []byte {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return nil
	}
	b, err := json.Marshal(f.Value)
	if err != nil {
		return nil
	}
	return b
}

// ---------------------------------------------------------------------------
// Comparison helpers (shared)
// ---------------------------------------------------------------------------

func strPtrEqualShared(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func stringSliceEqualShared(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func bytesEqualShared(a, b []byte) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func dateEqualShared(a, b pgtype.Date) bool {
	if !a.Valid && !b.Valid {
		return true
	}
	if a.Valid != b.Valid {
		return false
	}
	return a.Time.Equal(b.Time)
}

func numericEqualShared(a, b pgtype.Numeric) bool {
	if !a.Valid && !b.Valid {
		return true
	}
	if a.Valid != b.Valid {
		return false
	}
	return numericBigFloatShared(a).Cmp(numericBigFloatShared(b)) == 0
}

func numericBigFloatShared(n pgtype.Numeric) *big.Float {
	if !n.Valid || n.Int == nil {
		return new(big.Float)
	}
	f := new(big.Float).SetInt(n.Int)
	exp := int(n.Exp)
	base := big.NewFloat(1)
	ten := big.NewFloat(10)
	for i := 0; i < exp; i++ {
		base.Mul(base, ten)
	}
	for i := 0; i > exp; i-- {
		base.Quo(base, ten)
	}
	f.Mul(f, base)
	return f
}

// ---------------------------------------------------------------------------
// Editorial overrides + review log helpers (shared)
// ---------------------------------------------------------------------------

func parseOverrideKeysShared(raw []byte) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	if len(raw) == 0 {
		return out, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	for k := range m {
		out[k] = struct{}{}
	}
	return out, nil
}

// fieldChangeShared is one entry in the intel_review_log.field_changes JSON array.
type fieldChangeShared struct {
	Field string `json:"field"`
	From  any    `json:"from"`
	To    any    `json:"to"`
}

func marshalFieldChangesShared(changes []fieldChangeShared) ([]byte, error) {
	return json.Marshal(changes)
}

// ---------------------------------------------------------------------------
// Misc helpers (shared)
// ---------------------------------------------------------------------------

func ptrShared[T any](v T) *T { return &v }

func emptyJSONObjectShared() []byte { return []byte("{}") }

func isNotFoundShared(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no rows in result set")
}

func derefStrShared(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func dateStrShared(d pgtype.Date) any {
	if !d.Valid {
		return nil
	}
	return d.Time.Format("2006-01-02")
}

func numericStrShared(n pgtype.Numeric) any {
	if !n.Valid {
		return nil
	}
	return numericBigFloatShared(n).Text('f', -1)
}

func jsonOrNullShared(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return string(b)
	}
	return v
}

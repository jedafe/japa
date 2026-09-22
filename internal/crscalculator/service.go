// Package crscalculator — service.go holds the CRS scoring engine and
// orchestration for §107. Point values live in crs_factor_points (data);
// this file implements only the *matching* semantics — which descriptor
// shapes mean what — so an IRCC rules change ships as a new ruleset row,
// not a deploy.
//
// Transferability caps (max 50 per combination, 100 total) are CRS
// structural rules, not point values, so they live here. If IRCC ever
// changes them, this file changes — a ruleset row alone cannot express
// them.
// // Ref: japaconnect-engineering/sections/107-crs-calculator.md
// //	japaconnect-engineering/adr/ADR-118-crs-calculator.md
package crscalculator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// Transferability structural caps (IRCC fixed rules).
const (
	transferabilityCapPerCombo = 50
	transferabilityCapTotal    = 100
)

// educationRank orders education levels so `edu_min` conditions can be
// evaluated as "level >= min".
var educationRank = map[string]int{
	"none":          0,
	"secondary":     1,
	"one_year_post": 2,
	"two_year_post": 3,
	"bachelors":     4,
	"two_or_more":   5,
	"masters":       6,
	"professional":  7,
	"phd":           8,
}

// CLBScores carries per-skill Canadian Language Benchmark scores.
type CLBScores struct {
	Listening int `json:"listening"`
	Reading   int `json:"reading"`
	Writing   int `json:"writing"`
	Speaking  int `json:"speaking"`
}

// Min returns the lowest of the four skill scores.
func (c CLBScores) Min() int {
	m := c.Listening
	for _, v := range []int{c.Reading, c.Writing, c.Speaking} {
		if v < m {
			m = v
		}
	}
	return m
}

// Each returns the four skills as a slice.
func (c CLBScores) Each() []int {
	return []int{c.Listening, c.Reading, c.Writing, c.Speaking}
}

// SpouseInput carries spouse/partner factors.
type SpouseInput struct {
	Education        string    `json:"education"`
	CLBFirst         CLBScores `json:"clb_first"`
	CanadianExpYears int       `json:"canadian_experience_years"`
}

// CalculateRequest is the POST /crs/calculate profile.
type CalculateRequest struct {
	MaritalStatus              string       `json:"marital_status"` // single|married
	Age                        int          `json:"age"`
	Education                  string       `json:"education"`
	CLBFirst                   CLBScores    `json:"clb_first"`
	CLBSecond                  *CLBScores   `json:"clb_second,omitempty"`
	CanadianExpYears           int          `json:"canadian_experience_years"`
	ForeignExpYears            int          `json:"foreign_experience_years"`
	CertificateOfQualification bool         `json:"certificate_of_qualification"`
	Spouse                     *SpouseInput `json:"spouse,omitempty"`
	ProvincialNomination       bool         `json:"provincial_nomination"`
	JobOffer                   bool         `json:"job_offer"`
	SiblingInCanada            bool         `json:"sibling_in_canada"`
	FrenchCLB                  *CLBScores   `json:"french_clb,omitempty"`
	CanadianEducation          string       `json:"canadian_education"` // none|one_or_two_years|three_plus_years
	RulesetVersion             string       `json:"ruleset_version,omitempty"`
}

// BreakdownItem is one awarded factor in the score breakdown.
type BreakdownItem struct {
	Group  string `json:"group"`
	Key    string `json:"key"`
	Label  string `json:"label"`
	Points int32  `json:"points"`
}

// CalculateResult is the POST /crs/calculate response.
type CalculateResult struct {
	RulesetVersion string          `json:"ruleset_version"`
	Score          int32           `json:"score"`
	Breakdown      []BreakdownItem `json:"breakdown"`
	Saved          bool            `json:"saved"` // history row persisted (authed only)
}

// RulesetDTO is the API shape for a ruleset.
type RulesetDTO struct {
	ID            string `json:"id"`
	VersionLabel  string `json:"version_label"`
	EffectiveDate string `json:"effective_date"`
	Status        string `json:"status"`
	SourceURL     string `json:"source_url,omitempty"`
}

// FactorDTO is the API shape for one factor row.
type FactorDTO struct {
	Group            string          `json:"group"`
	Key              string          `json:"key"`
	InputDescriptor  json.RawMessage `json:"input_descriptor"`
	Label            string          `json:"label"`
	PointsSingle     int32           `json:"points_single"`
	PointsWithSpouse *int32          `json:"points_with_spouse,omitempty"`
	SortOrder        int32           `json:"sort_order"`
}

// FactorsResponse is GET /crs/factors.
type FactorsResponse struct {
	Ruleset RulesetDTO  `json:"ruleset"`
	Factors []FactorDTO `json:"factors"`
}

// DrawDTO is the API shape for one draw.
type DrawDTO struct {
	ID                string `json:"id"`
	DrawNumber        *int32 `json:"draw_number,omitempty"`
	DrawDate          string `json:"draw_date"`
	Category          string `json:"category"`
	CRSCutoff         int32  `json:"crs_cutoff"`
	InvitationsIssued int32  `json:"invitations_issued"`
	TieBreakRule      string `json:"tie_break_rule,omitempty"`
	SourceURL         string `json:"source_url,omitempty"`
}

// ScoreHistoryDTO is the API shape for a saved score.
type ScoreHistoryDTO struct {
	ID        string          `json:"id"`
	Score     int32           `json:"score"`
	Inputs    json.RawMessage `json:"inputs"`
	Breakdown json.RawMessage `json:"breakdown"`
	CreatedAt string          `json:"created_at"`
}

// Admin request bodies.
type CreateRulesetRequest struct {
	VersionLabel  string           `json:"version_label"`
	EffectiveDate string           `json:"effective_date"`
	SourceURL     string           `json:"source_url"`
	Factors       []FactorRowInput `json:"factors"`
}

// FactorRowInput is one factor row in a create-ruleset request.
type FactorRowInput struct {
	FactorGroup      string          `json:"factor_group"`
	FactorKey        string          `json:"factor_key"`
	InputDescriptor  json.RawMessage `json:"input_descriptor"`
	Label            string          `json:"label"`
	PointsSingle     int32           `json:"points_single"`
	PointsWithSpouse *int32          `json:"points_with_spouse"`
	SortOrder        int32           `json:"sort_order"`
}

// DrawRequest is the admin body for draw create/update.
type DrawRequest struct {
	DrawNumber        *int32  `json:"draw_number"`
	DrawDate          string  `json:"draw_date"`
	Category          string  `json:"category"`
	CRSCutoff         *int32  `json:"crs_cutoff"`
	InvitationsIssued *int32  `json:"invitations_issued"`
	TieBreakRule      *string `json:"tie_break_rule"`
	SourceURL         *string `json:"source_url"`
}

// Service orchestrates the CRS calculator.
type Service struct {
	store    *Store
	settings *config.Settings
}

// NewService creates the service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// GetFactors returns the factor table for the requested version (or the
// active ruleset when version is empty).
func (s *Service) GetFactors(ctx context.Context, version string) (*FactorsResponse, error) {
	ruleset, err := s.resolveRuleset(ctx, version)
	if err != nil {
		return nil, err
	}

	rows, err := s.store.ListFactorPoints(ctx, ruleset.ID)
	if err != nil {
		return nil, err
	}

	factors := make([]FactorDTO, 0, len(rows))
	for _, f := range rows {
		d := FactorDTO{
			Group:           f.FactorGroup,
			Key:             f.FactorKey,
			InputDescriptor: json.RawMessage(f.InputDescriptor),
			Label:           f.Label,
			PointsSingle:    f.PointsSingle,
			SortOrder:       f.SortOrder,
		}
		if f.PointsWithSpouse.Valid {
			v := f.PointsWithSpouse.Int32
			d.PointsWithSpouse = &v
		}
		factors = append(factors, d)
	}

	return &FactorsResponse{Ruleset: rulesetDTO(ruleset), Factors: factors}, nil
}

// Calculate scores a profile and persists history when the caller is
// authenticated.
func (s *Service) Calculate(ctx context.Context, userID *pgtype.UUID, in CalculateRequest) (*CalculateResult, error) {
	if err := validateProfile(in); err != nil {
		return nil, err
	}

	ruleset, err := s.resolveRuleset(ctx, in.RulesetVersion)
	if err != nil {
		return nil, err
	}

	factors, err := s.store.ListFactorPoints(ctx, ruleset.ID)
	if err != nil {
		return nil, err
	}

	score, breakdown := scoreProfile(factors, in)

	result := &CalculateResult{
		RulesetVersion: ruleset.VersionLabel,
		Score:          score,
		Breakdown:      breakdown,
	}

	if userID != nil {
		inputs, _ := json.Marshal(in)
		bd, _ := json.Marshal(breakdown)
		if _, err := s.store.SaveScoreHistory(ctx, *userID, ruleset.ID, inputs, bd, score); err == nil {
			result.Saved = true
		}
	}

	return result, nil
}

// ListDraws returns published draw history.
func (s *Service) ListDraws(ctx context.Context, category string, limit int32) ([]DrawDTO, error) {
	rows, err := s.store.ListDraws(ctx, category, limit)
	if err != nil {
		return nil, err
	}
	return drawDTOs(rows), nil
}

// LatestDraws returns the most recent draw per category.
func (s *Service) LatestDraws(ctx context.Context) ([]DrawDTO, error) {
	rows, err := s.store.LatestDraws(ctx)
	if err != nil {
		return nil, err
	}
	return drawDTOs(rows), nil
}

// ScoreHistory returns a user's saved scores.
func (s *Service) ScoreHistory(ctx context.Context, userID pgtype.UUID, page, limit int32) ([]ScoreHistoryDTO, int64, error) {
	rows, total, err := s.store.ListScoreHistory(ctx, userID, page, limit)
	if err != nil {
		return nil, 0, err
	}

	out := make([]ScoreHistoryDTO, 0, len(rows))
	for _, h := range rows {
		out = append(out, ScoreHistoryDTO{
			ID:        uuidStr(h.ID),
			Score:     h.Score,
			Inputs:    json.RawMessage(h.Inputs),
			Breakdown: json.RawMessage(h.Breakdown),
			CreatedAt: h.CreatedAt.Time.Format(time.RFC3339),
		})
	}
	return out, total, nil
}

// ListRulesets returns all rulesets (admin).
func (s *Service) ListRulesets(ctx context.Context) ([]RulesetDTO, error) {
	rows, err := s.store.ListRulesets(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RulesetDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, rulesetDTO(r))
	}
	return out, nil
}

// CreateRuleset creates a draft ruleset with its factor rows in one tx.
func (s *Service) CreateRuleset(ctx context.Context, in CreateRulesetRequest) (*RulesetDTO, error) {
	if in.VersionLabel == "" || in.EffectiveDate == "" {
		return nil, errors.New("version_label and effective_date are required")
	}
	var d pgtype.Date
	if err := d.Scan(in.EffectiveDate); err != nil {
		return nil, fmt.Errorf("invalid effective_date")
	}

	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id, err := s.store.CreateRuleset(ctx, tx, in.VersionLabel, d, in.SourceURL)
	if err != nil {
		return nil, err
	}

	for i, f := range in.Factors {
		row := FactorRow{
			RulesetID:       id,
			FactorGroup:     f.FactorGroup,
			FactorKey:       f.FactorKey,
			InputDescriptor: f.InputDescriptor,
			Label:           f.Label,
			PointsSingle:    f.PointsSingle,
			SortOrder:       int32(i), // default to array order
		}
		if f.SortOrder != 0 {
			row.SortOrder = f.SortOrder
		}
		if f.PointsWithSpouse != nil {
			row.PointsWithSpouse = pgtype.Int4{Int32: *f.PointsWithSpouse, Valid: true}
		}
		if err := s.store.CreateFactorPoint(ctx, tx, row); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &RulesetDTO{ID: uuidStr(id), VersionLabel: in.VersionLabel, EffectiveDate: in.EffectiveDate, Status: RulesetDraft, SourceURL: in.SourceURL}, nil
}

// ApproveRuleset activates a ruleset and supersedes the prior active one.
func (s *Service) ApproveRuleset(ctx context.Context, id, reviewerID pgtype.UUID) error {
	return s.store.ApproveRuleset(ctx, id, reviewerID)
}

// CreateDraw records a draw entry (admin).
func (s *Service) CreateDraw(ctx context.Context, in DrawRequest) (*DrawDTO, error) {
	if in.DrawDate == "" || in.Category == "" || in.CRSCutoff == nil || in.InvitationsIssued == nil {
		return nil, errors.New("draw_date, category, crs_cutoff and invitations_issued are required")
	}
	var d pgtype.Date
	if err := d.Scan(in.DrawDate); err != nil {
		return nil, fmt.Errorf("invalid draw_date")
	}

	row := DrawRow{
		DrawDate:          d,
		Category:          in.Category,
		CRSCutoff:         *in.CRSCutoff,
		InvitationsIssued: *in.InvitationsIssued,
		TieBreakRule:      in.TieBreakRule,
		SourceURL:         in.SourceURL,
	}
	if in.DrawNumber != nil {
		row.DrawNumber = pgtype.Int4{Int32: *in.DrawNumber, Valid: true}
	}

	id, err := s.store.CreateDraw(ctx, row)
	if err != nil {
		return nil, err
	}
	row.ID = id
	dtos := drawDTOs([]DrawRow{row})
	return &dtos[0], nil
}

// UpdateDraw patches a draw entry (admin).
func (s *Service) UpdateDraw(ctx context.Context, id pgtype.UUID, in DrawRequest) error {
	row := DrawRow{}
	if in.DrawNumber != nil {
		row.DrawNumber = pgtype.Int4{Int32: *in.DrawNumber, Valid: true}
	}
	if in.DrawDate != "" {
		var d pgtype.Date
		if err := d.Scan(in.DrawDate); err != nil {
			return fmt.Errorf("invalid draw_date")
		}
		row.DrawDate = d
	}
	if in.Category != "" {
		row.Category = in.Category
	}
	if in.CRSCutoff != nil {
		row.CRSCutoff = *in.CRSCutoff
	}
	if in.InvitationsIssued != nil {
		row.InvitationsIssued = *in.InvitationsIssued
	}
	row.TieBreakRule = in.TieBreakRule
	row.SourceURL = in.SourceURL
	return s.store.UpdateDraw(ctx, id, row)
}

// resolveRuleset picks the requested version or the active ruleset.
func (s *Service) resolveRuleset(ctx context.Context, version string) (RulesetRow, error) {
	if version != "" {
		return s.store.GetRulesetByVersion(ctx, version)
	}
	return s.store.GetActiveRuleset(ctx)
}

// validateProfile sanity-checks the submitted profile.
func validateProfile(in CalculateRequest) error {
	if in.MaritalStatus != "single" && in.MaritalStatus != "married" {
		return fmt.Errorf("marital_status must be single or married")
	}
	if in.Age < 0 || in.Age > 120 {
		return fmt.Errorf("age out of range")
	}
	if _, ok := educationRank[in.Education]; !ok {
		return fmt.Errorf("unknown education level")
	}
	for _, v := range in.CLBFirst.Each() {
		if v < 0 || v > 10 {
			return fmt.Errorf("clb_first scores must be 0–10")
		}
	}
	if in.MaritalStatus == "married" && in.Spouse == nil {
		return fmt.Errorf("spouse details required when married")
	}
	return nil
}

// ---------------------------------------------------------------------
// Scoring engine — descriptor matching
// ---------------------------------------------------------------------

// descriptor is the decoded input_descriptor jsonb.
type descriptor map[string]json.RawMessage

func (d descriptor) num(key string) (float64, bool) {
	raw, ok := d[key]
	if !ok {
		return 0, false
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, false
	}
	return n, true
}

func (d descriptor) str(key string) (string, bool) {
	raw, ok := d[key]
	if !ok {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

func (d descriptor) flag(key string) (bool, bool) {
	raw, ok := d[key]
	if !ok {
		return false, false
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return false, false
	}
	return b, true
}

func decodeDescriptor(raw []byte) descriptor {
	var d descriptor
	_ = json.Unmarshal(raw, &d)
	return d
}

// scoreProfile walks the factor table and awards points. Points come from
// the rows; the *meaning* of each descriptor field is implemented here.
func scoreProfile(factors []FactorRow, in CalculateRequest) (int32, []BreakdownItem) {
	married := in.MaritalStatus == "married"
	var total int32
	var breakdown []BreakdownItem

	award := func(group, key, label string, pts int32) {
		if pts <= 0 {
			return
		}
		breakdown = append(breakdown, BreakdownItem{Group: group, Key: key, Label: label, Points: pts})
		total += pts
	}

	pointsFor := func(f FactorRow) int32 {
		if married && f.PointsWithSpouse.Valid {
			return f.PointsWithSpouse.Int32
		}
		return f.PointsSingle
	}

	// --- core + spouse + additional: row-level matching ---
	comboScores := map[string]int32{} // transferability combo name → awarded points

	for _, f := range factors {
		d := decodeDescriptor(f.InputDescriptor)

		switch f.FactorKey {
		case "age":
			if inBand(d, "min_age", "max_age", float64(in.Age)) {
				award(f.FactorGroup, f.FactorKey, f.Label, pointsFor(f))
			}
		case "education", "spouse_education":
			level := in.Education
			if f.FactorKey == "spouse_education" {
				if !married || in.Spouse == nil {
					continue
				}
				level = in.Spouse.Education
			}
			if edu, ok := d.str("edu"); ok && edu == level {
				award(f.FactorGroup, f.FactorKey, f.Label, pointsFor(f))
			}
		case "language_first":
			for _, skill := range in.CLBFirst.Each() {
				if clb, ok := d.num("clb"); ok && int(clb) == skill {
					award(f.FactorGroup, f.FactorKey, f.Label, pointsFor(f))
				}
			}
		case "language_second":
			if in.CLBSecond == nil {
				continue
			}
			for _, skill := range in.CLBSecond.Each() {
				if clb, ok := d.num("clb"); ok && int(clb) == skill {
					award(f.FactorGroup, f.FactorKey, f.Label, pointsFor(f))
				}
			}
		case "spouse_language":
			if !married || in.Spouse == nil {
				continue
			}
			for _, skill := range in.Spouse.CLBFirst.Each() {
				if clb, ok := d.num("clb"); ok && int(clb) == skill {
					award(f.FactorGroup, f.FactorKey, f.Label, pointsFor(f))
				}
			}
		case "canadian_experience":
			if inBand(d, "min_years", "max_years", float64(in.CanadianExpYears)) {
				award(f.FactorGroup, f.FactorKey, f.Label, pointsFor(f))
			}
		case "spouse_canadian_experience":
			if !married || in.Spouse == nil {
				continue
			}
			if inBand(d, "min_years", "max_years", float64(in.Spouse.CanadianExpYears)) {
				award(f.FactorGroup, f.FactorKey, f.Label, pointsFor(f))
			}
		case "provincial_nomination":
			if v, _ := d.flag("flag"); v && in.ProvincialNomination {
				award(f.FactorGroup, f.FactorKey, f.Label, f.PointsSingle)
			}
		case "job_offer":
			if v, _ := d.flag("flag"); v && in.JobOffer {
				award(f.FactorGroup, f.FactorKey, f.Label, f.PointsSingle)
			}
		case "sibling":
			if v, _ := d.flag("flag"); v && in.SiblingInCanada {
				award(f.FactorGroup, f.FactorKey, f.Label, f.PointsSingle)
			}
		case "french":
			if in.FrenchCLB == nil {
				continue
			}
			if min, ok := d.num("clb_min"); ok && float64(in.FrenchCLB.Min()) >= min {
				award(f.FactorGroup, f.FactorKey, f.Label, f.PointsSingle)
			}
		case "canadian_education":
			if edu, ok := d.str("edu"); ok && edu == in.CanadianEducation {
				award(f.FactorGroup, f.FactorKey, f.Label, f.PointsSingle)
			}
		default:
			// transferability rows: descriptor carries a "combo" name plus
			// the conditions; award if all conditions pass.
			if f.FactorGroup != GroupTransferability {
				continue
			}
			combo, _ := d.str("combo")
			if combo == "" {
				combo = f.FactorKey
			}
			if transferabilityMatch(d, in) {
				if comboScores[combo]+f.PointsSingle > transferabilityCapPerCombo {
					f.PointsSingle = transferabilityCapPerCombo - comboScores[combo]
				}
				comboScores[combo] += f.PointsSingle
				award(f.FactorGroup, f.FactorKey, f.Label, f.PointsSingle)
			}
		}
	}

	// Enforce the 100-point transferability total cap by trimming the last
	// awarded items if needed (rows are sorted; overflow is clamped).
	if transferabilityTotal(breakdown) > transferabilityCapTotal {
		excess := transferabilityTotal(breakdown) - transferabilityCapTotal
		for i := len(breakdown) - 1; i >= 0 && excess > 0; i-- {
			if breakdown[i].Group != GroupTransferability {
				continue
			}
			trim := min32(excess, breakdown[i].Points)
			breakdown[i].Points -= trim
			total -= trim
			excess -= trim
		}
	}

	return total, breakdown
}

// inBand checks min <= v <= max using the descriptor's min_/max_ fields.
func inBand(d descriptor, minKey, maxKey string, v float64) bool {
	min, okMin := d.num(minKey)
	max, okMax := d.num(maxKey)
	if !okMin && !okMax {
		return false
	}
	if okMin && v < min {
		return false
	}
	if okMax && v > max {
		return false
	}
	return true
}

// transferabilityMatch evaluates the conditions inside a transferability
// descriptor: edu_min, clb_min, foreign_min, canadian_min, cert.
func transferabilityMatch(d descriptor, in CalculateRequest) bool {
	if eduMin, ok := d.str("edu_min"); ok {
		want := educationRank[eduMin]
		got := educationRank[in.Education]
		if got < want {
			return false
		}
	}
	if clbMin, ok := d.num("clb_min"); ok {
		if float64(in.CLBFirst.Min()) < clbMin {
			return false
		}
	}
	if foreignMin, ok := d.num("foreign_min"); ok {
		if float64(in.ForeignExpYears) < foreignMin {
			return false
		}
	}
	if canadianMin, ok := d.num("canadian_min"); ok {
		if float64(in.CanadianExpYears) < canadianMin {
			return false
		}
	}
	if want, ok := d.flag("cert"); ok && want && !in.CertificateOfQualification {
		return false
	}
	return true
}

func transferabilityTotal(b []BreakdownItem) int32 {
	var t int32
	for _, it := range b {
		if it.Group == GroupTransferability {
			t += it.Points
		}
	}
	return t
}

func min32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

// ---------------------------------------------------------------------
// DTO helpers
// ---------------------------------------------------------------------

func rulesetDTO(r RulesetRow) RulesetDTO {
	d := RulesetDTO{
		ID:           uuidStr(r.ID),
		VersionLabel: r.VersionLabel,
		Status:       r.Status,
	}
	if r.EffectiveDate.Valid {
		d.EffectiveDate = r.EffectiveDate.Time.Format("2006-01-02")
	}
	if r.SourceURL != nil {
		d.SourceURL = *r.SourceURL
	}
	return d
}

func drawDTOs(rows []DrawRow) []DrawDTO {
	out := make([]DrawDTO, 0, len(rows))
	for _, d := range rows {
		dto := DrawDTO{
			ID:                uuidStr(d.ID),
			DrawDate:          d.DrawDate.Time.Format("2006-01-02"),
			Category:          d.Category,
			CRSCutoff:         d.CRSCutoff,
			InvitationsIssued: d.InvitationsIssued,
		}
		if d.DrawNumber.Valid {
			n := d.DrawNumber.Int32
			dto.DrawNumber = &n
		}
		if d.TieBreakRule != nil {
			dto.TieBreakRule = *d.TieBreakRule
		}
		if d.SourceURL != nil {
			dto.SourceURL = *d.SourceURL
		}
		out = append(out, dto)
	}
	return out
}

func uuidStr(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// RejectRuleset marks a draft ruleset rejected (HITL reject from the
// unified moderation queue).
func (s *Service) RejectRuleset(ctx context.Context, id, reviewerID pgtype.UUID, notes string) error {
	return s.store.RejectRuleset(ctx, id, reviewerID, notes)
}

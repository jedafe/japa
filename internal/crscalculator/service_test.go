package crscalculator

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// factor builds a FactorRow with a jsonb descriptor literal.
func factor(group, key, descriptor, label string, single int32, withSpouse *int32) FactorRow {
	f := FactorRow{
		FactorGroup:     group,
		FactorKey:       key,
		InputDescriptor: []byte(descriptor),
		Label:           label,
		PointsSingle:    single,
	}
	if withSpouse != nil {
		f.PointsWithSpouse = pgtype.Int4{Int32: *withSpouse, Valid: true}
	}
	return f
}

func sp(v int32) *int32 { return &v }

// testFactors is a minimal but representative factor table exercising
// every descriptor shape the engine understands.
func testFactors() []FactorRow {
	return []FactorRow{
		// age bands (single / with-spouse variants)
		factor(GroupCore, "age", `{"min_age":20,"max_age":29}`, "Age 20-29", 110, sp(100)),
		factor(GroupCore, "age", `{"min_age":30,"max_age":39}`, "Age 30-39", 95, sp(88)),
		// education
		factor(GroupCore, "education", `{"edu":"bachelors"}`, "Bachelor's", 120, sp(112)),
		factor(GroupCore, "education", `{"edu":"masters"}`, "Master's", 135, sp(126)),
		// first official language — per skill (CLB 9 → 31 pts each, single)
		factor(GroupCore, "language_first", `{"clb":9}`, "CLB 9", 31, sp(29)),
		factor(GroupCore, "language_first", `{"clb":10}`, "CLB 10", 34, sp(32)),
		// second official language
		factor(GroupCore, "language_second", `{"clb":7}`, "2nd lang CLB 7", 3, nil),
		// Canadian experience band
		factor(GroupCore, "canadian_experience", `{"min_years":1,"max_years":1}`, "1yr Canadian exp", 40, sp(35)),
		factor(GroupCore, "canadian_experience", `{"min_years":2,"max_years":2}`, "2yr Canadian exp", 53, sp(46)),
		// spouse factors
		factor(GroupSpouse, "spouse_education", `{"edu":"bachelors"}`, "Spouse bachelor's", 8, nil),
		factor(GroupSpouse, "spouse_language", `{"clb":9}`, "Spouse CLB 9", 5, nil),
		factor(GroupSpouse, "spouse_canadian_experience", `{"min_years":1,"max_years":1}`, "Spouse 1yr exp", 5, nil),
		// transferability — two combos
		factor(GroupTransferability, "edu_language", `{"combo":"edu_lang","edu_min":"bachelors","clb_min":9}`, "Edu + strong language", 50, nil),
		factor(GroupTransferability, "foreign_language", `{"combo":"foreign_lang","foreign_min":1,"clb_min":7}`, "Foreign exp + language", 25, nil),
		factor(GroupTransferability, "foreign_language_hi", `{"combo":"foreign_lang","foreign_min":3,"clb_min":9}`, "3yr foreign + CLB9", 50, nil),
		// additional
		factor(GroupAdditional, "provincial_nomination", `{"flag":true}`, "PNP nomination", 600, nil),
		factor(GroupAdditional, "sibling", `{"flag":true}`, "Sibling in Canada", 15, nil),
		factor(GroupAdditional, "french", `{"clb_min":7}`, "French CLB 7+", 30, nil),
		factor(GroupAdditional, "canadian_education", `{"edu":"three_plus_years"}`, "3+yr Canadian study", 30, nil),
	}
}

func findPoints(b []BreakdownItem, group, key string) int32 {
	var total int32
	for _, it := range b {
		if it.Group == group && it.Key == key {
			total += it.Points
		}
	}
	return total
}

func TestScoreProfile_SingleProfile(t *testing.T) {
	in := CalculateRequest{
		MaritalStatus:    "single",
		Age:              29,
		Education:        "masters",
		CLBFirst:         CLBScores{Listening: 9, Reading: 9, Writing: 9, Speaking: 9},
		CanadianExpYears: 2,
		ForeignExpYears:  3,
		SiblingInCanada:  true,
	}

	score, breakdown := scoreProfile(testFactors(), in)

	// core: 110 age + 135 edu + 4×31 language + 53 canadian exp
	wantCore := int32(110 + 135 + 124 + 53)
	if got := findPoints(breakdown, GroupCore, "age"); got != 110 {
		t.Fatalf("age points = %d, want 110", got)
	}
	if got := findPoints(breakdown, GroupCore, "language_first"); got != 124 {
		t.Fatalf("language_first points = %d, want 124 (4×31)", got)
	}
	// transferability: edu_lang 50 + foreign_lang 25+50=75→ capped at 50/combo = 100 total cap
	if got := transferabilityTotal(breakdown); got != 100 {
		t.Fatalf("transferability total = %d, want 100 (capped)", got)
	}
	// additional: sibling 15
	if got := findPoints(breakdown, GroupAdditional, "sibling"); got != 15 {
		t.Fatalf("sibling points = %d, want 15", got)
	}
	if score != wantCore+100+15 {
		t.Fatalf("total = %d, want %d", score, wantCore+100+15)
	}
}

func TestScoreProfile_MarriedUsesSpousePoints(t *testing.T) {
	in := CalculateRequest{
		MaritalStatus:    "married",
		Age:              35,
		Education:        "bachelors",
		CLBFirst:         CLBScores{Listening: 10, Reading: 10, Writing: 10, Speaking: 10},
		CanadianExpYears: 1,
		Spouse: &SpouseInput{
			Education:        "bachelors",
			CLBFirst:         CLBScores{Listening: 9, Reading: 9, Writing: 9, Speaking: 9},
			CanadianExpYears: 1,
		},
	}

	score, breakdown := scoreProfile(testFactors(), in)

	if got := findPoints(breakdown, GroupCore, "age"); got != 88 {
		t.Fatalf("married age points = %d, want 88 (with-spouse variant)", got)
	}
	if got := findPoints(breakdown, GroupCore, "language_first"); got != 128 {
		t.Fatalf("married language = %d, want 128 (4×32)", got)
	}
	if got := findPoints(breakdown, GroupSpouse, "spouse_education"); got != 8 {
		t.Fatalf("spouse edu = %d, want 8", got)
	}
	if got := findPoints(breakdown, GroupSpouse, "spouse_language"); got != 20 {
		t.Fatalf("spouse language = %d, want 20 (4×5)", got)
	}
	// edu_lang combo needs edu_min bachelors ✓ but clb_min 9 ✓ → 50
	if got := findPoints(breakdown, GroupTransferability, "edu_language"); got != 50 {
		t.Fatalf("edu_language = %d, want 50", got)
	}
	if score <= 0 {
		t.Fatal("expected positive score")
	}
}

func TestScoreProfile_PNPAndAdditional(t *testing.T) {
	in := CalculateRequest{
		MaritalStatus:        "single",
		Age:                  22,
		Education:            "bachelors",
		CLBFirst:             CLBScores{Listening: 9, Reading: 9, Writing: 9, Speaking: 9},
		ProvincialNomination: true,
		FrenchCLB:            &CLBScores{Listening: 7, Reading: 8, Writing: 7, Speaking: 9},
		CanadianEducation:    "three_plus_years",
	}

	_, breakdown := scoreProfile(testFactors(), in)

	if got := findPoints(breakdown, GroupAdditional, "provincial_nomination"); got != 600 {
		t.Fatalf("PNP = %d, want 600", got)
	}
	if got := findPoints(breakdown, GroupAdditional, "french"); got != 30 {
		t.Fatalf("french = %d, want 30", got)
	}
	if got := findPoints(breakdown, GroupAdditional, "canadian_education"); got != 30 {
		t.Fatalf("canadian_education = %d, want 30", got)
	}
}

func TestScoreProfile_NoMatchNoPoints(t *testing.T) {
	in := CalculateRequest{
		MaritalStatus: "single",
		Age:           50,    // outside all bands
		Education:     "phd", // no phd row in fixture
		CLBFirst:      CLBScores{Listening: 4, Reading: 4, Writing: 4, Speaking: 4},
	}

	score, breakdown := scoreProfile(testFactors(), in)
	if score != 0 || len(breakdown) != 0 {
		t.Fatalf("expected 0 score with no matches, got %d (%v)", score, breakdown)
	}
}

func TestValidateProfile(t *testing.T) {
	cases := []struct {
		name    string
		in      CalculateRequest
		wantErr bool
	}{
		{"valid single", CalculateRequest{MaritalStatus: "single", Age: 30, Education: "bachelors"}, false},
		{"bad marital", CalculateRequest{MaritalStatus: "x", Age: 30, Education: "bachelors"}, true},
		{"bad edu", CalculateRequest{MaritalStatus: "single", Age: 30, Education: "wizard"}, true},
		{"bad clb", CalculateRequest{MaritalStatus: "single", Age: 30, Education: "bachelors", CLBFirst: CLBScores{Listening: 11}}, true},
		{"married no spouse", CalculateRequest{MaritalStatus: "married", Age: 30, Education: "bachelors"}, true},
		{"married ok", CalculateRequest{MaritalStatus: "married", Age: 30, Education: "bachelors", Spouse: &SpouseInput{Education: "bachelors"}}, false},
	}
	for _, tc := range cases {
		if err := validateProfile(tc.in); (err != nil) != tc.wantErr {
			t.Fatalf("%s: err=%v wantErr=%v", tc.name, err, tc.wantErr)
		}
	}
}

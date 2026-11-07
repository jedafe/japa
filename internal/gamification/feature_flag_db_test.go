package gamification

// DB integration coverage for §42 feature_flag filtering on badges and
// challenges (migration 001086): public catalogue, badge-collection stats,
// challenge lists/progress, and evaluation-time skips.
// Runs when JC_TEST_DATABASE_URL (falls back to JC_DATABASE_URL) points at a
// migrated database; skips otherwise — same convention as
// internal/interactions/block_db_test.go.

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/features"
)

func flagTestDB(t *testing.T) *db.DB {
	t.Helper()
	dsn := os.Getenv("JC_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("JC_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("JC_TEST_DATABASE_URL not set — skipping DB integration test")
	}
	d, err := db.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("cannot connect to test DB: %v", err)
	}
	t.Cleanup(func() { d.Writer.Close(); d.Reader.Close() })
	return d
}

func flagTestService(d *db.DB, checker FeatureChecker) *Service {
	s := NewService(NewStore(d), nil)
	s.SetFeatureChecker(checker)
	return s
}

func flagTestUser(t *testing.T, ctx context.Context, d *db.DB, tag string) pgtype.UUID {
	t.Helper()
	uname := fmt.Sprintf("flagtester_%s_%d", tag, time.Now().UnixNano())
	var id pgtype.UUID
	err := d.Writer.QueryRow(ctx,
		`INSERT INTO users (username, email) VALUES ($1, $2) RETURNING id`,
		uname, uname+"@test.dev").Scan(&id)
	if err != nil {
		t.Fatalf("inserting user: %v", err)
	}
	if _, err := d.Writer.Exec(ctx,
		`INSERT INTO user_profiles (user_id) VALUES ($1) ON CONFLICT DO NOTHING`, id); err != nil {
		t.Fatalf("inserting profile: %v", err)
	}
	t.Cleanup(func() {
		d.Writer.Exec(context.Background(), `DELETE FROM activity_events WHERE user_id = $1`, id)
		d.Writer.Exec(context.Background(), `DELETE FROM user_challenge_progress WHERE user_id = $1`, id)
		d.Writer.Exec(context.Background(), `DELETE FROM user_badges WHERE user_id = $1`, id)
		d.Writer.Exec(context.Background(), `DELETE FROM badge_progress WHERE user_id = $1`, id)
		d.Writer.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id)
	})
	return id
}

// flagTestSlug returns a run-unique slug so repeated test runs cannot
// collide on the definitions' unique constraint.
func flagTestSlug(prefix string) string {
	return fmt.Sprintf("fftest_%s_%d", prefix, time.Now().UnixNano()%1_000_000_000)
}

func createFlagTestBadge(t *testing.T, ctx context.Context, d *db.DB, slug string, flag *string, secret bool) gen.BadgeDefinition {
	t.Helper()
	b, err := gen.New(d.Writer).CreateBadgeDefinition(ctx, gen.CreateBadgeDefinitionParams{
		Slug:         slug,
		Name:         "FF test " + slug,
		TriggerType:  TriggerTypeManual,
		IsActive:     true,
		DisplayOrder: 0,
		Rarity:       "common",
		IsSecret:     secret,
		FeatureFlag:  flag,
	})
	if err != nil {
		t.Fatalf("creating badge %s: %v", slug, err)
	}
	t.Cleanup(func() {
		d.Writer.Exec(context.Background(), `DELETE FROM badge_progress WHERE badge_id = $1`, b.ID)
		d.Writer.Exec(context.Background(), `DELETE FROM user_badges WHERE badge_id = $1`, b.ID)
		d.Writer.Exec(context.Background(), `DELETE FROM badge_definitions WHERE id = $1`, b.ID)
	})
	return b
}

func createCountBadge(t *testing.T, ctx context.Context, d *db.DB, slug, activityKey string, flag *string) gen.BadgeDefinition {
	t.Helper()
	threshold := int32(1)
	b, err := gen.New(d.Writer).CreateBadgeDefinition(ctx, gen.CreateBadgeDefinitionParams{
		Slug:               slug,
		Name:               "FF test " + slug,
		TriggerType:        TriggerTypeActivityCount,
		TriggerValue:       &threshold,
		TriggerActivityKey: &activityKey,
		IsActive:           true,
		DisplayOrder:       0,
		Rarity:             "common",
		FeatureFlag:        flag,
	})
	if err != nil {
		t.Fatalf("creating count badge %s: %v", slug, err)
	}
	t.Cleanup(func() {
		d.Writer.Exec(context.Background(), `DELETE FROM badge_progress WHERE badge_id = $1`, b.ID)
		d.Writer.Exec(context.Background(), `DELETE FROM user_badges WHERE badge_id = $1`, b.ID)
		d.Writer.Exec(context.Background(), `DELETE FROM badge_definitions WHERE id = $1`, b.ID)
	})
	return b
}

func createFlagTestChallenge(t *testing.T, ctx context.Context, d *db.DB, slug, activityKey string, target int32, flag *string) gen.ChallengeDefinition {
	t.Helper()
	c, err := gen.New(d.Writer).CreateChallenge(ctx, gen.CreateChallengeParams{
		Slug:        slug,
		Name:        "FF test " + slug,
		ActivityKey: activityKey,
		TargetCount: target,
		StartsAt:    pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		IsActive:    true,
		FeatureFlag: flag,
	})
	if err != nil {
		t.Fatalf("creating challenge %s: %v", slug, err)
	}
	t.Cleanup(func() {
		d.Writer.Exec(context.Background(), `DELETE FROM user_challenge_progress WHERE challenge_id = $1`, c.ID)
		d.Writer.Exec(context.Background(), `DELETE FROM challenge_definitions WHERE id = $1`, c.ID)
	})
	return c
}

func insertActivityEvent(t *testing.T, ctx context.Context, d *db.DB, userID pgtype.UUID, key string) {
	t.Helper()
	_, err := d.Writer.Exec(ctx,
		`INSERT INTO activity_events (user_id, activity_key, reference) VALUES ($1, $2, $3)`,
		userID, key, fmt.Sprintf("%s/%d", key, time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("inserting activity event: %v", err)
	}
}

func badgeInfoSlugs(list []BadgeInfo) map[string]BadgeInfo {
	m := make(map[string]BadgeInfo, len(list))
	for _, b := range list {
		m[b.Slug] = b
	}
	return m
}

func challengeSlugs(list []ChallengeInfo) map[string]bool {
	m := make(map[string]bool, len(list))
	for _, c := range list {
		m[c.Slug] = true
	}
	return m
}

var flagTestScope = features.CallerScope{Surface: "main"}

// TestDBFeatureFlagBadgeCatalogue proves ListBadges hides flag-disabled
// definitions, keeps enabled and unflagged ones, masks feature_flag on
// unearned secret badges, and honours caller-scoped flag visibility.
func TestDBFeatureFlagBadgeCatalogue(t *testing.T) {
	d := flagTestDB(t)
	ctx := context.Background()

	onFlag := "ff_test.on"
	offFlag := "ff_test.off"
	scopedFlag := "ff_test.scoped"

	onSlug := flagTestSlug("on")
	createFlagTestBadge(t, ctx, d, onSlug, &onFlag, false)
	offSlug := flagTestSlug("off")
	createFlagTestBadge(t, ctx, d, offSlug, &offFlag, false)
	nullSlug := flagTestSlug("null")
	createFlagTestBadge(t, ctx, d, nullSlug, nil, false)
	offSecretSlug := flagTestSlug("offsec")
	createFlagTestBadge(t, ctx, d, offSecretSlug, &offFlag, true)
	secretOnSlug := flagTestSlug("secon")
	createFlagTestBadge(t, ctx, d, secretOnSlug, &onFlag, true)
	scopedSlug := flagTestSlug("scoped")
	createFlagTestBadge(t, ctx, d, scopedSlug, &scopedFlag, false)

	s := flagTestService(d, fakeFeatureChecker{
		enabled: map[string]bool{onFlag: true, offFlag: false, scopedFlag: true},
		evalResults: map[string]features.EvalResult{
			// Enabled globally but invisible to this caller's scope —
			// surface/geo/audience pruning must hide it too.
			scopedFlag: {Enabled: true, Visible: false},
		},
	})

	got, err := s.ListBadges(ctx, flagTestScope)
	if err != nil {
		t.Fatalf("ListBadges: %v", err)
	}
	bySlug := badgeInfoSlugs(got)

	if _, ok := bySlug[offSlug]; ok {
		t.Errorf("flag-off badge %s must be hidden", offSlug)
	}
	if _, ok := bySlug[offSecretSlug]; ok {
		t.Errorf("flag-off secret badge %s must be fully hidden (no ??? slot)", offSecretSlug)
	}
	if _, ok := bySlug[scopedSlug]; ok {
		t.Errorf("scope-invisible badge %s must be hidden", scopedSlug)
	}
	on, ok := bySlug[onSlug]
	if !ok {
		t.Fatalf("flag-on badge %s must be listed", onSlug)
	}
	if on.FeatureFlag == nil || *on.FeatureFlag != onFlag {
		t.Errorf("flag-on badge must expose its flag; got %v", on.FeatureFlag)
	}
	nullb, ok := bySlug[nullSlug]
	if !ok {
		t.Fatalf("unflagged badge %s must be listed", nullSlug)
	}
	if nullb.FeatureFlag != nil {
		t.Errorf("unflagged badge must have nil feature_flag; got %v", *nullb.FeatureFlag)
	}
	sec, ok := bySlug[secretOnSlug]
	if !ok {
		t.Fatalf("secret badge with enabled flag must be listed")
	}
	if sec.FeatureFlag != nil || sec.Name == "FF test "+secretOnSlug {
		t.Errorf("unearned secret badge must mask name and feature_flag; got name=%q flag=%v", sec.Name, sec.FeatureFlag)
	}
}

// TestDBFeatureFlagCollection proves GetBadgeCollection excludes
// flag-disabled definitions — including ones the user already earned —
// while ListUserBadges still returns them as earned history.
func TestDBFeatureFlagCollection(t *testing.T) {
	d := flagTestDB(t)
	ctx := context.Background()
	userID := flagTestUser(t, ctx, d, "collection")

	onFlag := "ff_test.col_on"
	offFlag := "ff_test.col_off"

	onSlug := flagTestSlug("con")
	onBadge := createFlagTestBadge(t, ctx, d, onSlug, &onFlag, false)
	offSlug := flagTestSlug("coff")
	offBadge := createFlagTestBadge(t, ctx, d, offSlug, &offFlag, false)
	earnedOffSlug := flagTestSlug("ceoff")
	earnedOffBadge := createFlagTestBadge(t, ctx, d, earnedOffSlug, &offFlag, false)

	s := flagTestService(d, fakeFeatureChecker{
		enabled: map[string]bool{onFlag: true, offFlag: false},
	})

	// Earn both the enabled and the disabled badge.
	for _, b := range []gen.BadgeDefinition{onBadge, offBadge, earnedOffBadge} {
		if _, err := d.Writer.Exec(ctx,
			`INSERT INTO user_badges (user_id, badge_id) VALUES ($1, $2)`, userID, b.ID); err != nil {
			t.Fatalf("awarding badge: %v", err)
		}
	}

	col, err := s.GetBadgeCollection(ctx, userID, flagTestScope)
	if err != nil {
		t.Fatalf("GetBadgeCollection: %v", err)
	}

	var found map[string]CollectionBadge = make(map[string]CollectionBadge)
	totalBadges := 0
	for _, cat := range col.Categories {
		for _, cb := range cat.Badges {
			found[cb.Slug] = cb
			totalBadges++
		}
	}
	if _, ok := found[offSlug]; ok {
		t.Errorf("flag-off badge %s must not appear in the collection", offSlug)
	}
	if _, ok := found[earnedOffSlug]; ok {
		t.Errorf("earned flag-off badge %s must not inflate the collection", earnedOffSlug)
	}
	cb, ok := found[onSlug]
	if !ok {
		t.Fatalf("flag-on badge %s must appear in the collection", onSlug)
	}
	if !cb.Earned {
		t.Errorf("flag-on badge should be marked earned")
	}
	if cb.FeatureFlag == nil || *cb.FeatureFlag != onFlag {
		t.Errorf("collection badge must carry its flag; got %v", cb.FeatureFlag)
	}
	if col.Stats.Total != totalBadges {
		t.Errorf("stats.total %d != returned badges %d", col.Stats.Total, totalBadges)
	}

	// Earned history keeps every award regardless of flag state.
	earned, err := s.ListUserBadges(ctx, userID)
	if err != nil {
		t.Fatalf("ListUserBadges: %v", err)
	}
	earnedSlugs := make(map[string]bool, len(earned))
	for _, b := range earned {
		earnedSlugs[b.Slug] = true
	}
	for _, slug := range []string{onSlug, offSlug, earnedOffSlug} {
		if !earnedSlugs[slug] {
			t.Errorf("earned badge %s must remain visible in ListUserBadges", slug)
		}
	}
}

// TestDBFeatureFlagChallenges proves flag-disabled challenges are hidden
// from the active list and from in-progress progress rows, while completed
// history stays visible.
func TestDBFeatureFlagChallenges(t *testing.T) {
	d := flagTestDB(t)
	ctx := context.Background()
	userID := flagTestUser(t, ctx, d, "chal")

	onFlag := "ff_test.ch_on"
	offFlag := "ff_test.ch_off"

	onSlug := flagTestSlug("chon")
	createFlagTestChallenge(t, ctx, d, onSlug, "fftest.ch_on_event", 3, &onFlag)
	offSlug := flagTestSlug("choff")
	offCh := createFlagTestChallenge(t, ctx, d, offSlug, "fftest.ch_off_event", 3, &offFlag)
	nullSlug := flagTestSlug("chnull")
	createFlagTestChallenge(t, ctx, d, nullSlug, "fftest.ch_null_event", 3, nil)

	s := flagTestService(d, fakeFeatureChecker{
		enabled: map[string]bool{onFlag: true, offFlag: false},
	})

	list, err := s.ListActiveChallenges(ctx, flagTestScope)
	if err != nil {
		t.Fatalf("ListActiveChallenges: %v", err)
	}
	slugs := challengeSlugs(list)
	if slugs[offSlug] {
		t.Errorf("flag-off challenge %s must be hidden from active list", offSlug)
	}
	if !slugs[onSlug] || !slugs[nullSlug] {
		t.Errorf("flag-on and unflagged challenges must be listed (on=%v null=%v)", slugs[onSlug], slugs[nullSlug])
	}

	// In-progress row on the disabled challenge → hidden.
	if _, err := d.Writer.Exec(ctx,
		`INSERT INTO user_challenge_progress (user_id, challenge_id, current_count, completed)
		 VALUES ($1, $2, 1, false)`, userID, offCh.ID); err != nil {
		t.Fatalf("inserting in-progress row: %v", err)
	}
	// Completed row on the same disabled challenge stays as earned history.
	offDoneSlug := flagTestSlug("chdone")
	offDoneCh := createFlagTestChallenge(t, ctx, d, offDoneSlug, "fftest.ch_done_event", 1, &offFlag)
	if _, err := d.Writer.Exec(ctx,
		`INSERT INTO user_challenge_progress (user_id, challenge_id, current_count, completed, completed_at)
		 VALUES ($1, $2, 1, true, now())`, userID, offDoneCh.ID); err != nil {
		t.Fatalf("inserting completed row: %v", err)
	}

	progress, err := s.ListUserChallengeProgress(ctx, userID, flagTestScope)
	if err != nil {
		t.Fatalf("ListUserChallengeProgress: %v", err)
	}
	bySlug := make(map[string]ChallengeWithProgress, len(progress))
	for _, p := range progress {
		bySlug[p.Slug] = p
	}
	if _, ok := bySlug[offSlug]; ok {
		t.Errorf("in-progress row on flag-off challenge must be hidden")
	}
	if p, ok := bySlug[offDoneSlug]; !ok || !p.Completed {
		t.Errorf("completed row on flag-off challenge must stay visible (found=%v)", ok)
	}
}

// TestDBFeatureFlagEvaluationSkip proves flag-disabled badges neither
// accrue progress nor award, and flag-disabled challenges do not accept
// progress — the evaluation-side guard behind §42.
func TestDBFeatureFlagEvaluationSkip(t *testing.T) {
	d := flagTestDB(t)
	ctx := context.Background()
	userID := flagTestUser(t, ctx, d, "eval")

	offFlag := "ff_test.ev_off"
	keyOff := "fftest.eval_off"
	keyOn := "fftest.eval_on"
	keyChalOff := "fftest.eval_chal_off"
	keyChalOn := "fftest.eval_chal_on"

	offBadgeSlug := flagTestSlug("evoff")
	offBadge := createCountBadge(t, ctx, d, offBadgeSlug, keyOff, &offFlag)
	onBadgeSlug := flagTestSlug("evon")
	createCountBadge(t, ctx, d, onBadgeSlug, keyOn, nil)

	createFlagTestChallenge(t, ctx, d, flagTestSlug("coff"), keyChalOff, 3, &offFlag)
	onChal := createFlagTestChallenge(t, ctx, d, flagTestSlug("con"), keyChalOn, 3, nil)

	s := flagTestService(d, fakeFeatureChecker{
		enabled: map[string]bool{offFlag: false},
	})

	for _, key := range []string{keyOff, keyOn, keyChalOff, keyChalOn} {
		insertActivityEvent(t, ctx, d, userID, key)
	}

	awarded, err := s.EvaluateBadges(ctx, userID, keyOff)
	if err != nil {
		t.Fatalf("EvaluateBadges(off): %v", err)
	}
	if len(awarded) != 0 {
		t.Errorf("flag-off badge must not award; got %d awards", len(awarded))
	}
	var n int
	if err := d.Writer.QueryRow(ctx,
		`SELECT COUNT(*) FROM badge_progress WHERE user_id = $1 AND badge_id = $2`,
		userID, offBadge.ID).Scan(&n); err != nil {
		t.Fatalf("checking badge_progress: %v", err)
	}
	if n != 0 {
		t.Errorf("flag-off badge must not accrue progress rows")
	}
	if err := d.Writer.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_badges WHERE user_id = $1 AND badge_id = $2`,
		userID, offBadge.ID).Scan(&n); err != nil {
		t.Fatalf("checking user_badges: %v", err)
	}
	if n != 0 {
		t.Errorf("flag-off badge must not appear in user_badges")
	}

	// Unflagged control badge proves the eval path itself works.
	awarded, err = s.EvaluateBadges(ctx, userID, keyOn)
	if err != nil {
		t.Fatalf("EvaluateBadges(on): %v", err)
	}
	if len(awarded) != 1 || awarded[0].Slug != onBadgeSlug {
		t.Errorf("unflagged badge should award; got %+v", awarded)
	}

	// Challenge progress: disabled challenge skipped, unflagged control accrues.
	if _, err := s.EvaluateBadges(ctx, userID, keyChalOff); err != nil {
		t.Fatalf("EvaluateBadges(chal_off): %v", err)
	}
	if _, err := s.EvaluateBadges(ctx, userID, keyChalOn); err != nil {
		t.Fatalf("EvaluateBadges(chal_on): %v", err)
	}
	var offChalID pgtype.UUID
	if err := d.Writer.QueryRow(ctx,
		`SELECT id FROM challenge_definitions WHERE activity_key = $1`, keyChalOff).Scan(&offChalID); err != nil {
		t.Fatalf("looking up off challenge: %v", err)
	}
	if err := d.Writer.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_challenge_progress WHERE user_id = $1 AND challenge_id = $2`,
		userID, offChalID).Scan(&n); err != nil {
		t.Fatalf("checking off-challenge progress: %v", err)
	}
	if n != 0 {
		t.Errorf("flag-off challenge must not accept progress")
	}
	var cnt int
	if err := d.Writer.QueryRow(ctx,
		`SELECT current_count FROM user_challenge_progress WHERE user_id = $1 AND challenge_id = $2`,
		userID, onChal.ID).Scan(&cnt); err != nil {
		t.Fatalf("unflagged challenge should have progress: %v", err)
	}
	if cnt != 1 {
		t.Errorf("expected control challenge current_count=1; got %d", cnt)
	}
}

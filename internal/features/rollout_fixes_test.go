package features

import (
	"fmt"
	"testing"
)

// TestDisplayPhase verifies a disabled flag can never report shipped — the
// explicit phase floors at "testing" while enabled=false.
func TestDisplayPhase(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		phase   string
		enabled bool
		want    string
	}{
		{"shipped enabled", "shipped", true, "shipped"},
		{"shipped disabled floors to testing", "shipped", false, "testing"},
		{"archived disabled stays archived", "archived", false, "archived"},
		{"planned disabled", "planned", false, "planned"},
		{"testing disabled", "testing", false, "testing"},
		{"in_progress disabled", "in_progress", false, "in_progress"},
	} {
		if got := DisplayPhase(tt.phase, tt.enabled); got != tt.want {
			t.Errorf("%s: DisplayPhase(%q, %v) = %q, want %q", tt.name, tt.phase, tt.enabled, got, tt.want)
		}
	}
}

// TestConfigIsInternal verifies the raw-config check used by the public
// projections — absent, malformed, and explicitly-false configs are public.
func TestConfigIsInternal(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		config  []byte
		want    bool
	}{
		{"nil config", nil, false},
		{"empty config", []byte{}, false},
		{"empty object", []byte(`{}`), false},
		{"internal true", []byte(`{"internal":true}`), true},
		{"internal false", []byte(`{"internal":false}`), false},
		{"internal alongside other fields", []byte(`{"phase":"shipped","internal":true}`), true},
		{"malformed json", []byte(`{bad`), false},
	} {
		if got := ConfigIsInternal(tt.config); got != tt.want {
			t.Errorf("%s: ConfigIsInternal = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestBucketID verifies the rollout bucketing identity: authenticated callers
// bucket per-user, everything else keeps the per-install platform bucket.
func TestBucketID(t *testing.T) {
	t.Parallel()

	c := New(nil, nil)

	// No subject → platform identifier (the historical behavior).
	if got := c.bucketID(t.Context(), CallerScope{}); got != c.rolloutID(t.Context()) {
		t.Errorf("anonymous bucketID = %q, want platform id %q", got, c.rolloutID(t.Context()))
	}

	// Subject set → per-user bucket distinct from the platform bucket.
	sc := CallerScope{SubjectID: "01234567-89ab-cdef-0123-456789abcdef"}
	got := c.bucketID(t.Context(), sc)
	want := "u:" + sc.SubjectID
	if got != want {
		t.Errorf("authed bucketID = %q, want %q", got, want)
	}
	if got == c.rolloutID(t.Context()) {
		t.Error("authed bucketID collided with platform bucket")
	}
}

// TestEvalFlag_PerUserRolloutBucketing seeds a flag at 50% rollout and
// verifies each caller's verdict follows their own hash bucket — distinct
// users land in different cohorts instead of sharing the platform bucket.
func TestEvalFlag_PerUserRolloutBucketing(t *testing.T) {
	t.Parallel()

	c := &Checker{cache: &flagCache{entries: make(map[string]*flagCacheEntry)}}
	c.cacheSet("feature.rollout_test", true, []byte(`{"rolloutPercentage":50}`))

	inCohort := func(subject string) bool {
		return hashInRollout("u:"+subject, "feature.rollout_test", 50)
	}

	// Find one subject in-cohort and one out — deterministic FNV buckets.
	var inUser, outUser string
	for i := 0; i < 100 && (inUser == "" || outUser == ""); i++ {
		u := "00000000-0000-4000-8000-" + fmt.Sprintf("%012d", i)
		switch {
		case inCohort(u) && inUser == "":
			inUser = u
		case !inCohort(u) && outUser == "":
			outUser = u
		}
	}
	if inUser == "" || outUser == "" {
		t.Fatal("could not find subjects in both cohorts")
	}

	if got := c.EvalFlag(t.Context(), "feature.rollout_test", CallerScope{SubjectID: inUser}).Enabled; !got {
		t.Errorf("in-cohort user got Enabled=false")
	}
	if got := c.EvalFlag(t.Context(), "feature.rollout_test", CallerScope{SubjectID: outUser}).Enabled; got {
		t.Errorf("out-cohort user got Enabled=true")
	}

	// Anonymous callers share the platform bucket — same verdict for all.
	a := c.EvalFlag(t.Context(), "feature.rollout_test", CallerScope{})
	b := c.EvalFlag(t.Context(), "feature.rollout_test", CallerScope{Surface: "mobile"})
	if a.Enabled != b.Enabled {
		t.Error("anonymous verdicts diverged across surfaces — expected shared platform bucket")
	}
}

package roadmap

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

func TestIsValidReleaseTransition(t *testing.T) {
	cases := []struct {
		from, to ReleaseStatus
		want     bool
	}{
		{ReleaseDraft, ReleasePublished, true},
		{ReleaseDraft, ReleaseYanked, false},
		{ReleasePublished, ReleaseYanked, true},
		{ReleasePublished, ReleaseDraft, false},
		{ReleaseYanked, ReleasePublished, true},
		{ReleaseYanked, ReleaseDraft, false},
		{ReleaseDraft, ReleaseDraft, true},
		{ReleasePublished, ReleasePublished, true},
	}
	for _, c := range cases {
		if got := IsValidReleaseTransition(c.from, c.to); got != c.want {
			t.Errorf("IsValidReleaseTransition(%q, %q) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestComputeSupportState(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	past := now.AddDate(0, -1, 0)
	future := now.AddDate(0, 1, 0)
	superseded := now.AddDate(0, -1, -1)

	ts := func(t time.Time) pgtype.Timestamptz {
		return pgtype.Timestamptz{Time: t, Valid: true}
	}

	cases := []struct {
		name       string
		release    gen.RoadmapRelease
		superseded *time.Time
		want       string
	}{
		{"draft", gen.RoadmapRelease{Status: "draft"}, nil, SupportDraft},
		{"yanked", gen.RoadmapRelease{Status: "yanked"}, nil, SupportYanked},
		{"upcoming", gen.RoadmapRelease{Status: "published", ReleasedAt: ts(future)}, nil, SupportUpcoming},
		{"unreleased_at", gen.RoadmapRelease{Status: "published"}, nil, SupportUpcoming},
		{"latest", gen.RoadmapRelease{Status: "published", ReleasedAt: ts(past)}, nil, SupportLatest},
		{"superseded→supported", gen.RoadmapRelease{Status: "published", ReleasedAt: ts(past)}, &superseded, SupportSupported},
		{"esr in window", gen.RoadmapRelease{Status: "published", ReleasedAt: ts(past), IsEsr: true, EolAt: ts(future)}, &superseded, SupportESR},
		{"esr no eol", gen.RoadmapRelease{Status: "published", ReleasedAt: ts(past), IsEsr: true}, &superseded, SupportESR},
		{"eol", gen.RoadmapRelease{Status: "published", ReleasedAt: ts(past), EolAt: ts(past)}, nil, SupportEOL},
		{"esr past eol → eol", gen.RoadmapRelease{Status: "published", ReleasedAt: ts(past), IsEsr: true, EolAt: ts(past)}, nil, SupportEOL},
	}
	for _, c := range cases {
		if got := ComputeSupportState(c.release, c.superseded, now); got != c.want {
			t.Errorf("%s: ComputeSupportState = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestParseConventionalCommit(t *testing.T) {
	cases := []struct {
		subject  string
		typ      string
		scope    string
		title    string
		breaking bool
	}{
		{"feat: add spaces composer", "feature", "", "add spaces composer", false},
		{"feat(blog): reactions rail", "feature", "blog", "reactions rail", false},
		{"fix(main): revert css split", "fix", "main", "revert css split", false},
		{"perf(main)!: merge CSS chunks", "performance", "main", "merge CSS chunks", true},
		{"feat!: breaking API change", "feature", "", "breaking API change", true},
		{"ui(feed): card polish", "ui", "feed", "card polish", false},
		{"ux: nav tweak", "ui", "", "nav tweak", false},
		{"style: lint pass", "ui", "", "lint pass", false},
		{"security: rotate keys", "security", "", "rotate keys", false},
		{"sec(auth): patch", "security", "auth", "patch", false},
		{"i18n: fr strings", "translation", "", "fr strings", false},
		{"a11y: label fixes", "accessibility", "", "label fixes", false},
		{"docs(jeos): spec update", "docs", "jeos", "spec update", false},
		{"refactor(store): split", "refactor", "store", "split", false},
		{"test: coverage", "test", "", "coverage", false},
		{"ci(deploy): tag lookup", "ci", "deploy", "tag lookup", false},
		{"chore: bump deps", "chore", "", "bump deps", false},
		{"revert: bad change", "chore", "", "bad change", false},
		{"random free text", "other", "", "random free text", false},
		{"wiki: something", "other", "", "something", false},
		{"", "other", "", "", false},
	}
	for _, c := range cases {
		typ, scope, title, breaking := ParseConventionalCommit(c.subject)
		if typ != c.typ || scope != c.scope || title != c.title || breaking != c.breaking {
			t.Errorf("ParseConventionalCommit(%q) = (%q,%q,%q,%v), want (%q,%q,%q,%v)",
				c.subject, typ, scope, title, breaking, c.typ, c.scope, c.title, c.breaking)
		}
	}
}

func TestMatchScopePaths(t *testing.T) {
	pathMap := map[string]string{
		"internal/blog":   "blog.enabled",
		"internal/forum":  "forum.enabled",
		"internal/spaces": "spaces.enabled",
		"apps/mobile":     "mobile",
	}
	cases := []struct {
		name  string
		files []string
		want  string
	}{
		{"blog file", []string{"internal/blog/handler.go"}, "blog.enabled"},
		{"blog dotfile", []string{"internal/blog.go"}, "blog.enabled"},
		{"blogfoo no match", []string{"internal/blogfoo/handler.go"}, ""},
		{"mobile", []string{"apps/mobile/lib/main.dart"}, "mobile"},
		{"longest prefix wins", []string{"internal/blog/api.go", "internal/x/y.go"}, "blog.enabled"},
		{"unrelated", []string{"docs/readme.md"}, ""},
		{"empty", []string{}, ""},
	}
	for _, c := range cases {
		if got := matchScopePaths(pathMap, c.files); got != c.want {
			t.Errorf("%s: matchScopePaths(%v) = %q, want %q", c.name, c.files, got, c.want)
		}
	}
}

func TestMobileSemverValidation(t *testing.T) {
	for _, v := range []string{"v2.4.0", "2.4.0", "v10.20.30"} {
		if !mobileSemverRe.MatchString(v) {
			t.Errorf("expected %q to match semver", v)
		}
	}
	for _, v := range []string{"2026.11", "November 2026", "v2", "abc"} {
		if mobileSemverRe.MatchString(v) {
			t.Errorf("expected %q to fail semver", v)
		}
	}
}

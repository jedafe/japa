package bots

import (
	"strings"
	"testing"
)

func TestGenerateRobotsTxt_BlockedBots(t *testing.T) {
	t.Parallel()

	bots := []KnownBot{
		{Name: "BadBot", UserAgentPattern: "BadBot", BotType: BotTypeScraper, Policy: PolicyBlock},
	}

	txt := GenerateRobotsTxt(bots)

	if !strings.Contains(txt, "User-agent: BadBot") {
		t.Error("expected User-agent directive for BadBot")
	}

	if !strings.Contains(txt, "Disallow: /") {
		t.Error("expected Disallow: / for blocked bot")
	}
}

func TestGenerateRobotsTxt_AllowedBots(t *testing.T) {
	t.Parallel()

	bots := []KnownBot{
		{Name: "Googlebot", UserAgentPattern: "Googlebot", BotType: BotTypeSearchEngine, Policy: PolicyAllow},
	}

	txt := GenerateRobotsTxt(bots)

	if !strings.Contains(txt, "User-agent: Googlebot") {
		t.Error("expected User-agent directive for Googlebot")
	}

	if !strings.Contains(txt, "Allow: /") {
		t.Error("expected Allow: / for allowed bot")
	}
}

func TestGenerateRobotsTxt_LimitedBots(t *testing.T) {
	t.Parallel()

	bots := []KnownBot{
		{Name: "SlowBot", UserAgentPattern: "SlowBot", BotType: BotTypeMonitoring, Policy: PolicyLimit, CrawlDelay: 10},
	}

	txt := GenerateRobotsTxt(bots)

	if !strings.Contains(txt, "User-agent: SlowBot") {
		t.Error("expected User-agent directive for SlowBot")
	}

	if !strings.Contains(txt, "Crawl-delay:") {
		t.Error("expected Crawl-delay for limited bot")
	}

	if !strings.Contains(txt, "Disallow: /admin/") {
		t.Error("expected Disallow: /admin/ for limited bot")
	}
}

func TestGenerateRobotsTxt_DefaultPolicy(t *testing.T) {
	t.Parallel()

	bots := []KnownBot{
		{Name: "Googlebot", UserAgentPattern: "Googlebot", BotType: BotTypeSearchEngine, Policy: PolicyAllow},
	}

	txt := GenerateRobotsTxt(bots)

	if !strings.Contains(txt, "User-agent: *") {
		t.Error("expected default User-agent: * directive")
	}

	if !strings.Contains(txt, "Sitemap: /sitemap.xml") {
		t.Error("expected Sitemap directive")
	}
}

func TestGenerateRobotsTxt_EmptyRegistry(t *testing.T) {
	t.Parallel()

	txt := GenerateRobotsTxt(nil)

	// Should still have default policy and sitemap
	if !strings.Contains(txt, "User-agent: *") {
		t.Error("expected default User-agent: * even with empty registry")
	}

	if !strings.Contains(txt, "Sitemap: /sitemap.xml") {
		t.Error("expected Sitemap directive even with empty registry")
	}
}

func TestGenerateRobotsTxt_MixedPolicies(t *testing.T) {
	t.Parallel()

	bots := []KnownBot{
		{Name: "Googlebot", UserAgentPattern: "Googlebot", BotType: BotTypeSearchEngine, Policy: PolicyAllow},
		{Name: "BadBot", UserAgentPattern: "BadBot", BotType: BotTypeScraper, Policy: PolicyBlock},
		{Name: "SlowBot", UserAgentPattern: "SlowBot", BotType: BotTypeMonitoring, Policy: PolicyLimit, CrawlDelay: 5},
	}

	txt := GenerateRobotsTxt(bots)

	// All three should appear
	if !strings.Contains(txt, "User-agent: BadBot") {
		t.Error("expected BadBot in output")
	}

	if !strings.Contains(txt, "User-agent: SlowBot") {
		t.Error("expected SlowBot in output")
	}

	if !strings.Contains(txt, "User-agent: Googlebot") {
		t.Error("expected Googlebot in output")
	}

	// Blocked should come before allowed (ordering check)
	badIdx := strings.Index(txt, "User-agent: BadBot")
	goodIdx := strings.Index(txt, "User-agent: Googlebot")

	if badIdx >= goodIdx {
		t.Error("blocked bots should appear before allowed bots")
	}
}

func TestGenerateRobotsTxt_CrawlDelayFormat(t *testing.T) {
	t.Parallel()

	bots := []KnownBot{
		{Name: "DelayedBot", UserAgentPattern: "DelayedBot", BotType: BotTypeMonitoring, Policy: PolicyLimit, CrawlDelay: 30},
	}

	txt := GenerateRobotsTxt(bots)

	if !strings.Contains(txt, "Crawl-delay: 30") {
		t.Errorf("expected Crawl-delay: 30 in output, got:\n%s", txt)
	}
}

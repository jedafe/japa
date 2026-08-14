package bots

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jedafe/japa/internal/db/gen"
)

func TestBotDetectionMiddleware_NoTenant(t *testing.T) {
	t.Parallel()

	store := &Store{}
	middleware := BotDetectionMiddleware(store, nil)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("User-Agent", "Googlebot/2.1")

	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	if info := BotInfoFromContext(req.Context()); info != nil {
		t.Error("expected nil bot info without tenant")
	}
}

func TestBotDetectionMiddleware_NoUserAgent(t *testing.T) {
	t.Parallel()

	store := &Store{}
	middleware := BotDetectionMiddleware(store, nil)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestBotInfoFromContext_Nil(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	if info := BotInfoFromContext(ctx); info != nil {
		t.Error("expected nil for empty context")
	}
}

func TestIsBotRequest(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	if IsBotRequest(ctx) {
		t.Error("expected false for empty context")
	}

	ctx = context.WithValue(ctx, BotInfoKey, &BotInfo{Name: "TestBot"})
	if !IsBotRequest(ctx) {
		t.Error("expected true when bot info present")
	}
}

func TestGetBotPolicy_Default(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	if p := GetBotPolicy(ctx); p != PolicyAllow {
		t.Errorf("expected default policy 'allow', got %q", p)
	}
}

func TestGetBotPolicy_FromContext(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(t.Context(), BotInfoKey, &BotInfo{Policy: PolicyBlock})
	if p := GetBotPolicy(ctx); p != PolicyBlock {
		t.Errorf("expected policy 'block', got %q", p)
	}
}

func TestFilterUserAgentsForLogs(t *testing.T) {
	t.Parallel()

	ua := "Mozilla/5.0 (compatible; Googlebot/2.1)"
	if got := FilterUserAgentsForLogs(ua); got != ua {
		t.Errorf("expected unchanged UA, got %q", got)
	}

	ua2 := "Mozilla/5.0\n<script>alert(1)</script>"

	got := FilterUserAgentsForLogs(ua2)
	if len(got) != len("Mozilla/5.0<script>alert(1)</script>") {
		t.Errorf("expected newlines removed, got %q", got)
	}

	longUA := make([]byte, 300)
	for i := range longUA {
		longUA[i] = 'a'
	}

	got = FilterUserAgentsForLogs(string(longUA))
	if len(got) != 256 {
		t.Errorf("expected truncated to 256, got %d", len(got))
	}
}

func TestMatchBotsByUserAgent(t *testing.T) {
	t.Parallel()

	bots := []gen.KnownBot{
		{Name: "Googlebot", UserAgentPattern: "Googlebot", BotType: "search_engine", Policy: "allow"},
		{Name: "GPTBot", UserAgentPattern: "GPTBot", BotType: "ai_llm", Policy: "allow"},
		{Name: "BadBot", UserAgentPattern: "BadBot", BotType: "scraper", Policy: "block"},
	}

	matched := matchBotsByUserAgent(bots, "Mozilla/5.0 (compatible; Googlebot/2.1)")
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}

	if matched[0].Name != "Googlebot" {
		t.Errorf("expected Googlebot, got %s", matched[0].Name)
	}

	matched = matchBotsByUserAgent(bots, "GPTBot/1.0")
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}

	if matched[0].Name != "GPTBot" {
		t.Errorf("expected GPTBot, got %s", matched[0].Name)
	}

	matched = matchBotsByUserAgent(bots, "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	if len(matched) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matched))
	}

	matched = matchBotsByUserAgent(bots, "googlebot/2.1")
	if len(matched) != 1 {
		t.Fatalf("expected 1 case-insensitive match, got %d", len(matched))
	}

	matched = matchBotsByUserAgent(bots, "GPTBot/1.0 (compatible; BadBot/1.0)")
	if len(matched) != 2 {
		t.Errorf("expected 2 matches, got %d", len(matched))
	}
}

func TestMatchBotsByUserAgent_SpecificityOrdering(t *testing.T) {
	t.Parallel()

	// "Bot" is a short generic pattern that matches many UAs; "Googlebot" is
	// more specific. The longer pattern must come first so the middleware
	// picks the most specific match.
	bots := []gen.KnownBot{
		{Name: "Generic Bot", UserAgentPattern: "Bot", BotType: "unknown", Policy: "allow"},
		{Name: "Googlebot", UserAgentPattern: "Googlebot", BotType: "search_engine", Policy: "allow"},
	}

	matched := matchBotsByUserAgent(bots, "Mozilla/5.0 (compatible; Googlebot/2.1)")
	if len(matched) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matched))
	}

	if matched[0].Name != "Googlebot" {
		t.Errorf("expected most specific (Googlebot) first, got %s", matched[0].Name)
	}

	if matched[1].Name != "Generic Bot" {
		t.Errorf("expected less specific (Generic Bot) second, got %s", matched[1].Name)
	}
}

func TestAllBotTypes_NoDuplicates(t *testing.T) {
	t.Parallel()

	types := AllBotTypes()
	seen := make(map[BotType]bool)

	for _, bt := range types {
		if seen[bt] {
			t.Errorf("duplicate bot type: %s", bt)
		}

		seen[bt] = true
	}
}

func TestAllPolicies_NoDuplicates(t *testing.T) {
	t.Parallel()

	policies := AllPolicies()
	seen := make(map[BotPolicy]bool)

	for _, p := range policies {
		if seen[p] {
			t.Errorf("duplicate policy: %s", p)
		}

		seen[p] = true
	}
}

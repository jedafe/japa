package sources

import "testing"

func TestTrustLevelConstants(t *testing.T) {
	if TrustLevelOfficial != 1 {
		t.Errorf("TrustLevelOfficial = %d, want 1", TrustLevelOfficial)
	}
	if TrustLevelTrusted != 2 {
		t.Errorf("TrustLevelTrusted = %d, want 2", TrustLevelTrusted)
	}
	if TrustLevelAggregator != 3 {
		t.Errorf("TrustLevelAggregator = %d, want 3", TrustLevelAggregator)
	}
	if TrustLevelCommunity != 4 {
		t.Errorf("TrustLevelCommunity = %d, want 4", TrustLevelCommunity)
	}
}

func TestCrawlMethodConstants(t *testing.T) {
	cases := map[CrawlMethod]string{
		CrawlMethodStatic: "static",
		CrawlMethodRSS:    "rss",
		CrawlMethodAPI:    "api",
		CrawlMethodPDF:    "pdf",
		CrawlMethodOCR:    "ocr",
	}
	for cm, want := range cases {
		if string(cm) != want {
			t.Errorf("got %q, want %q", cm, want)
		}
	}
}

func TestCrawlFrequencyConstants(t *testing.T) {
	cases := map[CrawlFrequency]string{
		CrawlFrequencyHourly:  "hourly",
		CrawlFrequencyDaily:   "daily",
		CrawlFrequencyWeekly:  "weekly",
		CrawlFrequencyMonthly: "monthly",
	}
	for cf, want := range cases {
		if string(cf) != want {
			t.Errorf("got %q, want %q", cf, want)
		}
	}
}

func TestCategoryConstants(t *testing.T) {
	cases := map[Category]string{
		CategoryScholarship: "scholarship",
		CategoryJob:         "job",
		CategoryVisa:        "visa",
		CategoryFX:          "fx",
		CategoryRemittance:  "remittance",
		CategoryNews:        "news",
		CategoryTravelAlert: "travel_alert",
	}
	for c, want := range cases {
		if string(c) != want {
			t.Errorf("got %q, want %q", c, want)
		}
	}
}

// TestCrawlNowStub verifies the CrawlNow stub returns the expected
// not-yet-wired error rather than panicking or silently succeeding.
func TestCrawlNowStub(t *testing.T) {
	svc := NewService(nil) // store is not touched by the stub
	err := svc.CrawlNow(t.Context(), SourceID{})
	if err == nil {
		t.Fatal("CrawlNow: expected error, got nil")
	}
	if !contains(err.Error(), "Task 22") {
		t.Errorf("CrawlNow error = %q, want substring %q", err.Error(), "Task 22")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

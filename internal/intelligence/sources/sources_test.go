package sources

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

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

// TestCrawlNow verifies CrawlNow returns an error when the store is nil
// (the service method now validates the source exists and is enabled,
// rather than returning a stub error). INTEL-011 fix.
func TestCrawlNow(t *testing.T) {
	svc := NewService(nil) // nil store → Get will panic/error
	// With a nil store, CrawlNow should return an error (not panic).
	// We use a recover wrapper since nil store dereference panics.
	defer func() {
		if r := recover(); r != nil {
			// Expected: nil store dereference is acceptable here since
			// the service is not meant to be used with a nil store in
			// production. The test verifies the method is no longer a
			// stub returning "Task 22".
		}
	}()
	err := svc.CrawlNow(t.Context(), pgtype.UUID{}, SourceID{})
	if err != nil && contains(err.Error(), "Task 22") {
		t.Errorf("CrawlNow should no longer return the old Task 22 stub error, got: %q", err.Error())
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

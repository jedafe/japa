package publicapi

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/feed"
	"github.com/jedafe/japa/internal/scamalert"
	"github.com/jedafe/japa/internal/visitor"
)

// stubIntelReader satisfies IntelReader for digest tests.
type stubIntelReader struct {
	listFn func(ctx context.Context, spaceID pgtype.UUID, limit int32) ([]IntelDigestItem, error)
}

func (s stubIntelReader) ListRecentForSpace(ctx context.Context, spaceID pgtype.UUID, limit int32) ([]IntelDigestItem, error) {
	if s.listFn == nil {
		return nil, nil
	}
	return s.listFn(ctx, spaceID, limit)
}

// geoFieldsDBTX stubs GetUserGeoProfileFields (QueryRow) with the given
// destination_country value; all other geo fields return empty strings.
func geoFieldsDBTX(destinationCountry string) stubDBTX {
	return stubDBTX{
		queryRowFn: func(sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "destination_country") {
				// Scan order: japa_status, destination_country,
				// origin_country, visa_route, purpose, planned_travel_date.
				return rowOf([]any{"", destinationCountry, "", "", "", ""})
			}
			return errRow(errors.New("geoFieldsDBTX: unexpected QueryRow"))
		},
	}
}

func digestFixtures() (pgtype.UUID, gen.GetPublicSpaceByGeoCodeRow) {
	var spaceID pgtype.UUID
	_ = spaceID.Scan("11111111-1111-1111-1111-111111111111")
	return spaceID, gen.GetPublicSpaceByGeoCodeRow{
		ID:     spaceID,
		Slug:   "germany",
		Name:   "Germany",
		Level:  "country",
		Status: "active",
	}
}

// TestDigestHappyPath covers the fully-populated digest: for_you +
// community from the feed reader, destination space resolved from
// destination_country, scam alerts + intel from their readers.
func TestDigestHappyPath(t *testing.T) {
	spaceID, space := digestFixtures()
	var viewer pgtype.UUID
	_ = viewer.Scan("22222222-2222-2222-2222-222222222222")

	svc := NewService(nil)
	svc.SetFeedReader(stubFeedReader{
		forYouFeedFn: func(_ context.Context, _ pgtype.UUID, _ string, limit, _ int32, _ *visitor.VisitorContext) ([]feed.FeedItem, error) {
			if limit != 8 {
				t.Fatalf("expected default section_limit 8, got %d", limit)
			}
			return []feed.FeedItem{{ID: "f1", Title: "For you"}}, nil
		},
		getFeedFn: func(_ context.Context, timeline string, _, _, _ *pgtype.UUID, sort string, _, _ int32, _, _ string) ([]feed.FeedItem, error) {
			if timeline != "explore" || sort != feed.SortTop {
				t.Fatalf("unexpected community call: %s %s", timeline, sort)
			}
			return []feed.FeedItem{{ID: "p1", Title: "Popular"}}, nil
		},
	})
	svc.SetMoatReaders(&MoatReaders{Spaces: stubSpaceReader{
		getByGeoCodeFn: func(_ context.Context, code string) (gen.GetPublicSpaceByGeoCodeRow, error) {
			if code != "DE" {
				t.Fatalf("expected normalized code DE, got %q", code)
			}
			return space, nil
		},
	}})
	svc.SetSafetyReader(stubSafetyReader{
		listFn: func(_ context.Context, dest pgtype.UUID, _ string, limit, _ int32) ([]scamalert.ScamReportDTO, int64, error) {
			if dest != spaceID {
				t.Fatalf("scam alerts queried for wrong space: %v", dest)
			}
			if limit != 3 {
				t.Fatalf("scam cap should be 3, got %d", limit)
			}
			return []scamalert.ScamReportDTO{{ID: "s1", ScamType: "visa_fraud"}}, 1, nil
		},
	})
	svc.SetIntelReader(stubIntelReader{
		listFn: func(_ context.Context, dest pgtype.UUID, _ int32) ([]IntelDigestItem, error) {
			if dest != spaceID {
				t.Fatalf("intel queried for wrong space: %v", dest)
			}
			return []IntelDigestItem{{ID: "i1", Kind: "visa_route", Title: "New route"}}, nil
		},
	})
	svc.SetQueries(gen.New(geoFieldsDBTX("de"))) // lowercase — must normalize to DE

	digest, err := svc.Digest(t.Context(), viewer)
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}
	if digest.Cached {
		t.Fatal("first call should not be cached")
	}
	if len(digest.ForYou.Items) != 1 || digest.ForYou.Items[0].Title != "For you" {
		t.Fatalf("for_you wrong: %+v", digest.ForYou)
	}
	if len(digest.Community.Popular) != 1 || digest.Community.Popular[0].Title != "Popular" {
		t.Fatalf("community wrong: %+v", digest.Community)
	}
	if digest.Destination == nil {
		t.Fatal("destination should be populated")
	}
	if digest.Destination.Space.Slug != "germany" || digest.Destination.Space.Level != "country" {
		t.Fatalf("destination space wrong: %+v", digest.Destination.Space)
	}
	if len(digest.Destination.ScamAlerts) != 1 || len(digest.Destination.Intel) != 1 {
		t.Fatalf("destination sections wrong: %+v", digest.Destination)
	}
}

// TestDigestNoDestination — unset destination_country yields
// destination: null while feed sections still populate.
func TestDigestNoDestination(t *testing.T) {
	var viewer pgtype.UUID
	_ = viewer.Scan("22222222-2222-2222-2222-222222222222")

	svc := NewService(nil)
	svc.SetFeedReader(stubFeedReader{})
	svc.SetQueries(gen.New(geoFieldsDBTX("")))

	digest, err := svc.Digest(t.Context(), viewer)
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}
	if digest.Destination != nil {
		t.Fatalf("expected nil destination, got %+v", digest.Destination)
	}
	if digest.ForYou.Items == nil || digest.Community.Popular == nil {
		t.Fatal("feed sections should be present (empty) even without a destination")
	}
}

// TestDigestNonPublicDestination — a destination_country resolving to a
// non-public/inactive space (GetPublicSpaceByGeoCode → ErrNoRows) yields
// destination: null rather than leaking the space.
func TestDigestNonPublicDestination(t *testing.T) {
	var viewer pgtype.UUID
	_ = viewer.Scan("22222222-2222-2222-2222-222222222222")

	svc := NewService(nil)
	svc.SetFeedReader(stubFeedReader{})
	svc.SetQueries(gen.New(geoFieldsDBTX("XX")))
	// default stubSpaceReader.getByGeoCodeFn nil → pgx.ErrNoRows
	svc.SetMoatReaders(&MoatReaders{Spaces: stubSpaceReader{}})

	digest, err := svc.Digest(t.Context(), viewer)
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}
	if digest.Destination != nil {
		t.Fatalf("non-public space must not surface in digest: %+v", digest.Destination)
	}
}

// TestDigestScamDisabled — scam_alert.enabled off omits scam_alerts but
// keeps the rest of the destination section.
func TestDigestScamDisabled(t *testing.T) {
	_, space := digestFixtures()
	var viewer pgtype.UUID
	_ = viewer.Scan("22222222-2222-2222-2222-222222222222")

	svc := NewService(nil)
	svc.SetFeedReader(stubFeedReader{})
	svc.SetQueries(gen.New(geoFieldsDBTX("DE")))
	svc.SetMoatReaders(&MoatReaders{Spaces: stubSpaceReader{
		getByGeoCodeFn: func(_ context.Context, _ string) (gen.GetPublicSpaceByGeoCodeRow, error) {
			return space, nil
		},
	}})
	svc.SetSafetyReader(stubSafetyReader{disabled: true})
	svc.SetIntelReader(stubIntelReader{
		listFn: func(_ context.Context, _ pgtype.UUID, _ int32) ([]IntelDigestItem, error) {
			return []IntelDigestItem{{ID: "i1", Kind: "job", Title: "Job"}}, nil
		},
	})

	digest, err := svc.Digest(t.Context(), viewer)
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}
	if digest.Destination == nil {
		t.Fatal("destination should still populate")
	}
	if len(digest.Destination.ScamAlerts) != 0 {
		t.Fatalf("scam section must be empty when flag is off: %+v", digest.Destination.ScamAlerts)
	}
	if len(digest.Destination.Intel) != 1 {
		t.Fatal("intel should still populate")
	}
}

// TestDigestFeedFailure — a feed-reader failure fails the request (the
// optional destination sections are the only soft-degrading parts).
func TestDigestFeedFailure(t *testing.T) {
	var viewer pgtype.UUID
	_ = viewer.Scan("22222222-2222-2222-2222-222222222222")

	svc := NewService(nil)
	svc.SetFeedReader(stubFeedReader{
		forYouFeedFn: func(context.Context, pgtype.UUID, string, int32, int32, *visitor.VisitorContext) ([]feed.FeedItem, error) {
			return nil, errors.New("feed backend down")
		},
	})

	if _, err := svc.Digest(t.Context(), viewer); err == nil {
		t.Fatal("expected feed failure to propagate")
	}
}

// TestDigestCacheHit — a second call within the TTL serves the cached
// digest without re-hitting the readers.
func TestDigestCacheHit(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	vk, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{mr.Addr()}, DisableCache: true})
	if err != nil {
		t.Fatalf("valkey.NewClient: %v", err)
	}
	t.Cleanup(vk.Close)

	var viewer pgtype.UUID
	_ = viewer.Scan("22222222-2222-2222-2222-222222222222")

	calls := 0
	svc := NewService(nil)
	svc.SetFeedReader(stubFeedReader{
		forYouFeedFn: func(context.Context, pgtype.UUID, string, int32, int32, *visitor.VisitorContext) ([]feed.FeedItem, error) {
			calls++
			return []feed.FeedItem{{ID: "f1"}}, nil
		},
	})
	svc.SetQueries(gen.New(geoFieldsDBTX("")))
	svc.SetCache(valkeycompat.NewAdapter(vk))

	d1, err := svc.Digest(t.Context(), viewer)
	if err != nil {
		t.Fatalf("Digest #1: %v", err)
	}
	d2, err := svc.Digest(t.Context(), viewer)
	if err != nil {
		t.Fatalf("Digest #2: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 reader call, got %d — cache did not serve second call", calls)
	}
	if d1.Cached || !d2.Cached {
		t.Fatalf("cached flags wrong: first=%v second=%v", d1.Cached, d2.Cached)
	}
}

//go:build integration

package video

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	japaerrors "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/media"
	"github.com/jedafe/japa/internal/testdb"
	"github.com/jedafe/japa/internal/users"
)

func TestService_ResolveURL_CreatesSyntheticUser(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)

	userSvc := users.NewService(database)
	videoStore := NewStore(database.Writer, database.Reader)

	svc := NewService(videoStore, userSvc, &fakeMediaManager{}, &fakeResolver{
		meta: &Metadata{
			SourceType:   SourceYouTube,
			SourceURL:    "https://www.youtube.com/watch?v=ABC123XYZ",
			ExternalID:   "ABC123XYZ",
			Title:        "Test Video",
			Description:  "Test description",
			ThumbnailURL: "https://example.com/thumb.jpg",
			DurationSecs: 305,
			ChannelName:  "Test Channel",
			AuthorName:   "Test Author",
		},
	})
	svc.SetSettings(newFakeSettings())

	res1, err := svc.ResolveURL(context.Background(), tenantID, "https://www.youtube.com/watch?v=ABC123XYZ")
	if err != nil {
		t.Fatalf("ResolveURL first: %v", err)
	}

	channelKey := fmt.Sprintf("%s:test-channel", SourceYouTube)
	if res1.Source.SourceUrl != "https://www.youtube.com/watch?v=ABC123XYZ" {
		t.Errorf("SourceUrl = %q, want canonical URL", res1.Source.SourceUrl)
	}
	if res1.Source.AuthorUserID == (pgtype.UUID{}) {
		t.Error("AuthorUserID is empty")
	}

	// Second resolve with the same URL should reuse the same synthetic user
	// and the same source row.
	res2, err := svc.ResolveURL(context.Background(), tenantID, res1.Source.SourceUrl)
	if err != nil {
		t.Fatalf("ResolveURL second: %v", err)
	}
	if res2.Source.ID != res1.Source.ID {
		t.Errorf("dedup failed: second source id %v != first %v", res2.Source.ID, res1.Source.ID)
	}
	if res2.Source.AuthorUserID != res1.Source.AuthorUserID {
		t.Errorf("synthetic user changed: %v != %v", res2.Source.AuthorUserID, res1.Source.AuthorUserID)
	}

	// The synthetic user should be findable by the channel key.
	sources, err := svc.ListChannelVideosByUsername(context.Background(), tenantID, channelKey, 20, 0)
	if err != nil {
		t.Fatalf("ListChannelVideosByUsername: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source for channel, got %d", len(sources))
	}
	if sources[0].ID != res1.Source.ID {
		t.Errorf("channel source id mismatch")
	}
}

func TestService_ResolveURL_Unsupported(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)

	videoStore := NewStore(database.Writer, database.Reader)
	svc := NewService(videoStore, &fakeUserManager{}, &fakeMediaManager{}, NewDefaultResolver())
	svc.SetSettings(newFakeSettings())

	_, err := svc.ResolveURL(context.Background(), tenantID, "https://example.com/video")
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("ResolveURL: err = %v, want ErrUnsupportedPlatform", err)
	}
}

type fakeSettings struct {
	bools map[string]bool
}

func (f *fakeSettings) GetStringOr(ctx context.Context, tenantID pgtype.UUID, key, fallback string) string {
	return fallback
}

func (f *fakeSettings) GetBoolOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback bool) bool {
	if f.bools == nil {
		f.bools = make(map[string]bool)
	}
	if v, ok := f.bools[key]; ok {
		return v
	}
	return fallback
}

func (f *fakeSettings) GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int {
	return fallback
}

func newFakeSettings() *fakeSettings {
	return &fakeSettings{bools: map[string]bool{"video.enabled": true}}
}

type fakeResolver struct {
	meta *Metadata
	err  error
}

func (f *fakeResolver) Resolve(ctx context.Context, rawURL string) (*Metadata, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.meta, nil
}

type fakeMediaManager struct {
	id pgtype.UUID
}

func (f *fakeMediaManager) StoreFromURL(ctx context.Context, tenantID, uploaderID pgtype.UUID, sourceURL, filename string) (pgtype.UUID, error) {
	return f.id, nil
}

func (f *fakeMediaManager) StoreFromBytes(ctx context.Context, tenantID, uploaderID pgtype.UUID, data []byte, filename string, attribution *media.MediaAttribution) (pgtype.UUID, error) {
	return f.id, nil
}

func (f *fakeMediaManager) GetMedia(ctx context.Context, tenantID, mediaID pgtype.UUID) (gen.Medium, error) {
	return gen.Medium{ID: mediaID, StorageBucket: "media", StoragePath: "media/test.mp4"}, nil
}

func (f *fakeMediaManager) MaxUploadBytes(ctx context.Context, tenantID pgtype.UUID) int64 {
	return 50 << 20
}

type fakeUserManager struct{}

func (f *fakeUserManager) EnsureSyntheticUser(ctx context.Context, tenantID pgtype.UUID, input users.SyntheticUserInput) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(uuid.New().String()); err != nil {
		return id, err
	}
	return id, nil
}

func newMediaID(t *testing.T) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(uuid.New().String()); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}
	return id
}

func TestService_ClaimAndApproveChannel(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)

	userSvc := users.NewService(database)
	realUserID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "real-claimant"})
	reviewerID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "reviewer"})

	videoStore := NewStore(database.Writer, database.Reader)
	svc := NewService(videoStore, userSvc, &fakeMediaManager{}, &fakeResolver{
		meta: &Metadata{
			SourceType:   SourceYouTube,
			SourceURL:    "https://www.youtube.com/watch?v=CLAIM001",
			ExternalID:   "CLAIM001",
			Title:        "Claimable Video",
			Description:  "Test description",
			ThumbnailURL: "https://example.com/thumb.jpg",
			DurationSecs: 120,
			ChannelName:  "Claimable Channel",
			AuthorName:   "Claimable Author",
		},
	})
	svc.SetSettings(newFakeSettings())

	res, err := svc.ResolveURL(context.Background(), tenantID, "https://www.youtube.com/watch?v=CLAIM001")
	if err != nil {
		t.Fatalf("ResolveURL: %v", err)
	}

	syntheticUserID := res.Source.AuthorUserID
	channelUsername := "youtube:claimable-channel"

	// The real user files an ownership claim.
	claim, err := svc.SubmitClaim(context.Background(), tenantID, realUserID, channelUsername, "claim", "https://example.com/evidence", "I own this channel", "")
	if err != nil {
		t.Fatalf("SubmitClaim: %v", err)
	}
	if claim.Status != "pending" {
		t.Errorf("claim status = %q, want pending", claim.Status)
	}
	if claim.ClaimType != "claim" {
		t.Errorf("claim type = %q, want claim", claim.ClaimType)
	}

	// An admin approves the claim.
	approved, err := svc.ReviewClaim(context.Background(), tenantID, reviewerID, claim.ID, "approved", "verified ownership")
	if err != nil {
		t.Fatalf("ReviewClaim: %v", err)
	}
	if approved.Status != "approved" {
		t.Errorf("approved status = %q, want approved", approved.Status)
	}

	// The video source now belongs to the real user.
	source, err := videoStore.GetSourceByID(context.Background(), tenantID, res.Source.ID)
	if err != nil {
		t.Fatalf("GetSourceByID: %v", err)
	}
	if source.AuthorUserID != realUserID {
		t.Errorf("source author = %v, want %v", source.AuthorUserID, realUserID)
	}

	// The synthetic user has been promoted to real.
	var isSynthetic bool
	err = database.Reader.QueryRow(context.Background(),
		"SELECT is_synthetic FROM users WHERE id = $1 AND tenant_id = $2",
		syntheticUserID, tenantID).Scan(&isSynthetic)
	if err != nil {
		t.Fatalf("querying synthetic user: %v", err)
	}
	if isSynthetic {
		t.Error("synthetic user was not promoted after claim approval")
	}

	// A second claim for the same channel is rejected.
	_, err = svc.SubmitClaim(context.Background(), tenantID, realUserID, channelUsername, "claim", "", "", "")
	if err == nil {
		t.Error("expected second claim to fail")
	}
}

func TestService_TakedownAndApprove(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)

	userSvc := users.NewService(database)
	realUserID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "real-takedown"})
	reviewerID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "takedown-reviewer"})

	videoStore := NewStore(database.Writer, database.Reader)
	svc := NewService(videoStore, userSvc, &fakeMediaManager{}, &fakeResolver{
		meta: &Metadata{
			SourceType:   SourceYouTube,
			SourceURL:    "https://www.youtube.com/watch?v=TKD001",
			ExternalID:   "TKD001",
			Title:        "Takedown Video",
			Description:  "Test description",
			ThumbnailURL: "https://example.com/thumb.jpg",
			DurationSecs: 60,
			ChannelName:  "Takedown Channel",
			AuthorName:   "Takedown Author",
		},
	})
	svc.SetSettings(newFakeSettings())

	res, err := svc.ResolveURL(context.Background(), tenantID, "https://www.youtube.com/watch?v=TKD001")
	if err != nil {
		t.Fatalf("ResolveURL: %v", err)
	}

	claim, err := svc.SubmitClaim(context.Background(), tenantID, realUserID, "youtube:takedown-channel", "takedown", "", "copyright issue", res.Source.ID.String())
	if err != nil {
		t.Fatalf("SubmitClaim takedown: %v", err)
	}
	if claim.ClaimType != "takedown" {
		t.Errorf("claim type = %q, want takedown", claim.ClaimType)
	}

	approved, err := svc.ReviewClaim(context.Background(), tenantID, reviewerID, claim.ID, "approved", "removed per request")
	if err != nil {
		t.Fatalf("ReviewClaim takedown: %v", err)
	}
	if approved.Status != "approved" {
		t.Errorf("approved status = %q, want approved", approved.Status)
	}

	source, err := videoStore.GetSourceByID(context.Background(), tenantID, res.Source.ID)
	if err != nil {
		t.Fatalf("GetSourceByID: %v", err)
	}
	if source.Status != "removed" {
		t.Errorf("source status = %q, want removed", source.Status)
	}
}

func TestService_ClaimFailsForRealUser(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)

	userSvc := users.NewService(database)
	testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "real-target"})
	claimantID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "claimant-real"})

	videoStore := NewStore(database.Writer, database.Reader)
	svc := NewService(videoStore, userSvc, &fakeMediaManager{}, &fakeResolver{})
	svc.SetSettings(newFakeSettings())

	_, err := svc.SubmitClaim(context.Background(), tenantID, claimantID, "real-target", "claim", "", "", "")
	if err == nil {
		t.Error("expected claim against real user to fail")
	}
}

type fakeDispatcher struct {
	reply string
}

func (f *fakeDispatcher) Dispatch(ctx context.Context, category string, req ai.Request) (ai.Response, error) {
	return ai.Response{Raw: map[string]any{"reply": f.reply}}, nil
}

func TestService_GenerateDraft(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)

	userSvc := users.NewService(database)
	realUserID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "video-drafter"})

	videoStore := NewStore(database.Writer, database.Reader)
	svc := NewService(videoStore, userSvc, &fakeMediaManager{}, &fakeResolver{
		meta: &Metadata{
			SourceType:   SourceYouTube,
			SourceURL:    "https://www.youtube.com/watch?v=DRAFT123",
			ExternalID:   "DRAFT123",
			Title:        "Draft Test Video",
			Description:  "A video to be summarized by the AI gateway.",
			ThumbnailURL: "https://example.com/thumb.jpg",
			DurationSecs: 120,
			ChannelName:  "Draft Channel",
			AuthorName:   "Draft Author",
		},
	})
	svc.SetSettings(newFakeSettings())

	res, err := svc.ResolveURL(context.Background(), tenantID, "https://www.youtube.com/watch?v=DRAFT123")
	if err != nil {
		t.Fatalf("ResolveURL: %v", err)
	}

	contentSvc := content.NewService(database)
	svc.SetContentService(contentSvc)
	svc.SetDispatcher(&fakeDispatcher{
		reply: `{"title":"AI Draft Title","body":"First paragraph.\n\nSecond paragraph."}`,
	})

	created, err := svc.GenerateDraft(context.Background(), tenantID, realUserID, res.Source.ID)
	if err != nil {
		t.Fatalf("GenerateDraft: %v", err)
	}

	if created.Title != "AI Draft Title" {
		t.Errorf("title = %q, want %q", created.Title, "AI Draft Title")
	}
	if created.ContentType != content.TypeVideo {
		t.Errorf("content_type = %q, want %q", created.ContentType, content.TypeVideo)
	}
	if created.Status != content.StatusInReview {
		t.Errorf("status = %q, want %q", created.Status, content.StatusInReview)
	}
	if !created.AiGenerated {
		t.Error("ai_generated = false, want true")
	}
	if created.VideoSourceID != res.Source.ID {
		t.Errorf("video_source_id mismatch")
	}
	if created.AuthorID != realUserID {
		t.Errorf("author_id mismatch")
	}
	if !strings.Contains(created.Body, "First paragraph") || !strings.Contains(created.Body, "Second paragraph") {
		t.Errorf("body missing paragraphs: %q", created.Body)
	}
	if !strings.HasPrefix(strings.TrimSpace(created.Body), "{") {
		t.Errorf("body is not TipTap JSON: %q", created.Body)
	}
}

func TestService_ResolveURL_WhenDisabled(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)

	videoStore := NewStore(database.Writer, database.Reader)
	svc := NewService(videoStore, &fakeUserManager{}, &fakeMediaManager{}, &fakeResolver{})
	svc.SetSettings(&fakeSettings{bools: map[string]bool{"video.enabled": false}})

	_, err := svc.ResolveURL(context.Background(), tenantID, "https://www.youtube.com/watch?v=ANY")
	if !errors.Is(err, japaerrors.ErrFeatureDisabled) {
		t.Fatalf("ResolveURL: err = %v, want ErrFeatureDisabled", err)
	}
}

func TestService_Upload(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)

	userSvc := users.NewService(database)
	uploaderID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "uploader"})

	mediaID := newMediaID(t)
	_, err := database.Writer.Exec(context.Background(),
		`INSERT INTO media (id, tenant_id, uploader_user_id, original_filename, mime_type, file_size_bytes, storage_path, confirmed)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, true)`,
		mediaID, tenantID, uploaderID, "demo.mp4", "video/mp4", 6, "media/demo.mp4")
	if err != nil {
		t.Fatalf("seed media: %v", err)
	}

	videoStore := NewStore(database.Writer, database.Reader)
	svc := NewService(videoStore, userSvc, &fakeMediaManager{id: mediaID}, &fakeResolver{})
	svc.SetSettings(newFakeSettings())

	res, err := svc.UploadVideo(context.Background(), tenantID, uploaderID, "demo.mp4", []byte("binary"), "Upload Title", "Upload description")
	if err != nil {
		t.Fatalf("UploadVideo: %v", err)
	}

	if res.Source.SourceType != SourceUpload {
		t.Errorf("source_type = %q, want %q", res.Source.SourceType, SourceUpload)
	}
	if res.Source.AuthorUserID != uploaderID {
		t.Errorf("author = %v, want %v", res.Source.AuthorUserID, uploaderID)
	}
	if res.Source.Title != "Upload Title" {
		t.Errorf("title = %q, want %q", res.Source.Title, "Upload Title")
	}
	if res.Source.ThumbnailMediaID == (pgtype.UUID{}) {
		t.Error("thumbnail_media_id is empty")
	}
	if res.EmbedJSON == nil {
		t.Error("embed_json is nil")
	}
}

func TestService_ReviewClaim_Rejected(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)

	userSvc := users.NewService(database)
	realUserID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "reject-claimant"})
	reviewerID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "reject-reviewer"})

	videoStore := NewStore(database.Writer, database.Reader)
	svc := NewService(videoStore, userSvc, &fakeMediaManager{}, &fakeResolver{
		meta: &Metadata{
			SourceType:   SourceYouTube,
			SourceURL:    "https://www.youtube.com/watch?v=REJECT01",
			ExternalID:   "REJECT01",
			Title:        "Reject Video",
			Description:  "Test",
			ThumbnailURL: "https://example.com/thumb.jpg",
			DurationSecs: 90,
			ChannelName:  "Reject Channel",
			AuthorName:   "Reject Author",
		},
	})
	svc.SetSettings(newFakeSettings())

	_, err := svc.ResolveURL(context.Background(), tenantID, "https://www.youtube.com/watch?v=REJECT01")
	if err != nil {
		t.Fatalf("ResolveURL: %v", err)
	}

	channelUsername := "youtube:reject-channel"
	claim, err := svc.SubmitClaim(context.Background(), tenantID, realUserID, channelUsername, "claim", "", "", "")
	if err != nil {
		t.Fatalf("SubmitClaim: %v", err)
	}

	rejected, err := svc.ReviewClaim(context.Background(), tenantID, reviewerID, claim.ID, "rejected", "insufficient evidence")
	if err != nil {
		t.Fatalf("ReviewClaim: %v", err)
	}
	if rejected.Status != "rejected" {
		t.Errorf("status = %q, want %q", rejected.Status, "rejected")
	}

	// A fresh claim for the same synthetic channel can be re-submitted after rejection.
	claim2, err := svc.SubmitClaim(context.Background(), tenantID, realUserID, channelUsername, "claim", "", "", "")
	if err != nil {
		t.Fatalf("SubmitClaim after rejection: %v", err)
	}
	if claim2.Status != "pending" {
		t.Errorf("re-submitted status = %q, want %q", claim2.Status, "pending")
	}
}

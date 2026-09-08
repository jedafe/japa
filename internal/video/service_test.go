//go:build integration

package video

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
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
		meta: &VideoMetadata{
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

	_, err := svc.ResolveURL(context.Background(), tenantID, "https://example.com/video")
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("ResolveURL: err = %v, want ErrUnsupportedPlatform", err)
	}
}

type fakeResolver struct {
	meta *VideoMetadata
	err  error
}

func (f *fakeResolver) Resolve(ctx context.Context, rawURL string) (*VideoMetadata, error) {
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
	return gen.Medium{ID: mediaID, TenantID: tenantID, StorageBucket: "media", StoragePath: "media/test.mp4"}, nil
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

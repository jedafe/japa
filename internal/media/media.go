// Package media implements the Media System (Section 34).
// Two-step upload: pre-sign → client uploads direct to SeaweedFS → confirm.
// imgproxy HMAC-SHA256 signed URLs generated server-side.
// Media Library at /dashboard/media with grid/list view.
// Modal Picker for TipTap and Page Builder integration.
package media

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// MIME type prefixes for filtering.
const (
	MIMEPrefixImage = "image"
	MIMEPrefixVideo = "video"
	MIMEPrefixAudio = "audio"
)

// Storage path constants.
const (
	StoragePathTemp  = "temp"
	StoragePathMedia = "media"
)

// DefaultBucket is the default bucket for SeaweedFS storage.
const DefaultBucket = "media"

// Pagination defaults.
const (
	DefaultPageSize int32 = 20
	MaxPageSize     int32 = 100
)

// SettingsProvider is the minimal interface the media service needs to read
// system_settings (Rule 3 — no hardcoded config). Implemented by
// *config.Settings.
type SettingsProvider interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
	GetStringOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback string) string
}

// MediaAttribution carries stock photo source metadata for attribution display.
type MediaAttribution struct {
	SourcePlatform        string
	SourcePhotographer    string
	SourcePhotographerURL string
	SourceLicense         string
	SourceLicenseURL      string
	SourcePhotoID         string
}

// Service provides media operations with reader/writer pool separation.
type Service struct {
	store    *Store
	db       *db.DB
	s3       *S3Client
	settings SettingsProvider
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		store: NewStore(gen.New(database.Reader), gen.New(database.Writer), database.Reader, database.Writer),
		db:    database,
		s3:    NewS3Client(),
	}
}

// SetSettings injects the system_settings provider for Rule 3 compliance
// (media.max_upload_mb, media.allowed_video_types, media.cdn.base_url).
func (s *Service) SetSettings(sp SettingsProvider) {
	s.settings = sp
}

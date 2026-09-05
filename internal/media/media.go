// Package media implements the Media System (Section 34).
// Two-step upload: pre-sign → client uploads direct to SeaweedFS → confirm.
// imgproxy HMAC-SHA256 signed URLs generated server-side.
// Media Library at /dashboard/media with grid/list view.
// Modal Picker for TipTap and Page Builder integration.
package media

import (
	"context"
	"os"

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
	GetStringOr(ctx context.Context, tenantID pgtype.UUID, key, fallback string) string
}

// MediaAttribution carries stock photo source metadata for attribution display.
//
//nolint:revive // exported: stutter naming is intentional
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
// The S3 client is built from JC_S3_* environment variables. Prefer
// NewServiceWithConfig when a config.Config is available (Rule 3).
func NewService(database *db.DB) *Service {
	return NewServiceWithConfig(database, S3Config{
		Endpoint:  os.Getenv("JC_S3_ENDPOINT"),
		AccessKey: os.Getenv("JC_S3_ACCESS_KEY"),
		SecretKey: os.Getenv("JC_S3_SECRET_KEY"),
		Region:    os.Getenv("JC_S3_REGION"),
		Bucket:    os.Getenv("JC_S3_BUCKET"),
	})
}

// NewServiceWithConfig creates a Service backed by the given database pool
// and an S3Config loaded by internal/config (Rule 3 — no ad-hoc os.Getenv).
// Pass media.S3Config fields from a *config.Config.
func NewServiceWithConfig(database *db.DB, s3cfg S3Config) *Service {
	return &Service{
		store: NewStore(gen.New(database.Reader), gen.New(database.Writer), database.Reader, database.Writer),
		db:    database,
		s3:    NewS3ClientFromConfig(s3cfg),
	}
}

// SetSettings injects the system_settings provider for Rule 3 compliance
// (media.max_upload_mb, media.allowed_video_types, media.cdn.base_url).
func (s *Service) SetSettings(sp SettingsProvider) {
	s.settings = sp
}

// ReconfigureS3FromSettings rebuilds the S3 client from the admin-configurable
// media.storage.* system_settings (Section 34 — CDN & S3-Compatible Object
// Storage). When the settings provider is nil or the endpoint/access_key/
// secret_key are empty, the existing env-var-based client (if any) is retained
// so callers degrade gracefully. Call once after SetSettings during server
// wiring; the settings are hot-reloadable so a subsequent call picks up
// Control Panel changes.
func (s *Service) ReconfigureS3FromSettings(ctx context.Context, tenantID pgtype.UUID) {
	if s.settings == nil {
		return
	}

	endpoint := s.settings.GetStringOr(ctx, tenantID, "media.storage.endpoint", "")
	accessKey := s.settings.GetStringOr(ctx, tenantID, "media.storage.access_key", "")
	secretKey := s.settings.GetStringOr(ctx, tenantID, "media.storage.secret_key", "")

	if endpoint == "" || accessKey == "" || secretKey == "" {
		// Not configured via settings — keep the env-var-based client.
		return
	}

	region := s.settings.GetStringOr(ctx, tenantID, "media.storage.region", "us-east-1")
	bucket := s.settings.GetStringOr(ctx, tenantID, "media.storage.bucket", DefaultBucket)
	s.s3 = NewS3ClientFromConfig(S3Config{
		Endpoint:  endpoint,
		AccessKey: accessKey,
		SecretKey: secretKey,
		Region:    region,
		Bucket:    bucket,
	})
}

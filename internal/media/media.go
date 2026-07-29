// Package media implements the Media System (Section 34).
// Two-step upload: pre-sign → client uploads direct to SeaweedFS → confirm.
// imgproxy HMAC-SHA256 signed URLs generated server-side.
// Media Library at /dashboard/media with grid/list view.
// Modal Picker for TipTap and Page Builder integration.
package media

import (
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
	store *Store
	db    *db.DB
	s3    *S3Client
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		store: NewStore(gen.New(database.Reader), gen.New(database.Writer), database.Reader, database.Writer),
		db:    database,
		s3:    NewS3Client(),
	}
}

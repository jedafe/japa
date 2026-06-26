// Package media implements the Media System (Section 34).
// Two-step upload: pre-sign → client uploads direct to MinIO → confirm.
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

// Default bucket for MinIO storage.
const DefaultBucket = "media"

// Pagination defaults.
const (
	DefaultPageSize int32 = 20
	MaxPageSize     int32 = 100
)

// Service provides media operations with reader/writer pool separation.
type Service struct {
	store *Store
	db    *db.DB
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		store: NewStore(gen.New(database.Reader), gen.New(database.Writer)),
		db:    database,
	}
}

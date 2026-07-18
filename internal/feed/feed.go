// Package feed implements the Activity Feed / Timeline system (Section 32).
// Four timelines: explore (guests), primary (logged-in), space, profile.
// Four sort modes: latest, top, rising, relevant.
// Hybrid fan-out: push for spaces ≤ 5000 members, pull for larger.
package feed

import (
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Sort mode constants for GetFeed.
const (
	SortLatest   = "latest"
	SortTop      = "top"
	SortRising   = "rising"
	SortRelevant = "relevant"
)

// FanOutThreshold is the member count at which fan-out switches from
// push (write feeds rows) to pull (followers pull on next request).
const FanOutThreshold = 5000

// DefaultPageSize is the default number of feed items returned per page.
const DefaultPageSize = 20

// MaxPageSize is the maximum number of feed items a client can request.
const MaxPageSize = 100

// Service provides feed operations with reader/writer pool separation.
type Service struct {
	store    *Store
	db       *db.DB
	enqueuer feedEnqueuer
	vk       valkeycompat.Cmdable
}

// feedEnqueuer is the subset of worker.Enqueuer that feed needs.
type feedEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// NewService creates a Service backed by the given database pool and Valkey
// client. The Valkey client caches the ranked For-You feed (may be nil, in
// which case ranking runs on every request).
func NewService(database *db.DB, vk valkeycompat.Cmdable) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
		db: database,
		vk: vk,
	}
}

// SetEnqueuer injects the Asynq enqueuer for async fan-out tasks.
func (s *Service) SetEnqueuer(e feedEnqueuer) {
	s.enqueuer = e
}

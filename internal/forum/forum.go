// Package forum implements the Forum Q&A / Solved system (Section 08).
//
// Nested threads with per-thread solved status, upvote/downvote voting,
// tag system with moderators, polls, anonymous posting, bookmarks,
// and reply view tracking. Solved threads are eligible for Google's
// Q&A rich-result structured-data treatment (Section 26).
package forum

import (
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Vote values for forum_threads and forum_replies.
const (
	VoteDown int16 = -1
	VoteUp   int16 = 1
)

// Entity types for bookmarks and votes.
const (
	EntityTypeThread = "thread"
	EntityTypeReply  = "reply"
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// DefaultEditWindowMinutes is the default editable window after posting.
const DefaultEditWindowMinutes = 30

// Service provides forum operations with reader/writer pool separation.
type Service struct {
	store    *Store
	db       *db.DB
	settings forumSettingsReader
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
			db:    database,
		},
		db: database,
	}
}

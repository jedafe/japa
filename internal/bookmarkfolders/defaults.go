// Package bookmarkfolders — first-class named bookmark folders.
//
// Ref: japaconnect-engineering/sections/08-forum-qa-solved.md (Bookmarks),
//
//	japaconnect-engineering/sections/50-user-dashboard-settings.md.
package bookmarkfolders

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// DefaultFolderResolver determines the default bookmark folder for any
// entity/content type and ensures the folder exists for a given user.
type DefaultFolderResolver struct {
	service *Service
}

// NewDefaultFolderResolver creates a resolver backed by the given Service.
func NewDefaultFolderResolver(service *Service) *DefaultFolderResolver {
	return &DefaultFolderResolver{service: service}
}

// contentTypeFolderNames maps contents.content_type values to default folder
// names. Kept separate from entityTypeFolderNames because content bookmarks
// are stored with entity_type "content" and require a content_type lookup.
var contentTypeFolderNames = map[string]string{
	"blog_post":       "blog",
	"discussion":      "discussions",
	"guide":           "guides",
	"page":            "pages",
	"comparison":      "comparisons",
	"event":           "events",
	"job_listing":     "jobs",
	"story":           "stories",
	"achievement":     "achievements",
	"service_listing": "services",
}

// entityTypeFolderNames maps entity_type values directly to default folder
// names, covering non-content entity types and normalized forum types.
var entityTypeFolderNames = map[string]string{
	"blog_post":      "blog",
	"academy_track": "learn",
	"doc_page":       "docs",
	"forum_thread":   "discussions",
	"forum_reply":    "discussions",
	"thread":         "discussions",
	"reply":          "discussions",
	"space":          "spaces",
	"tag":            "tags",
	"comment":        "comments",
}

// defaultFolderOrder is the canonical order of default folders. It is used
// both for pre-seeding at registration and for assigning position to lazy
// folders created on first bookmark.
var defaultFolderOrder = []string{
	"blog", "discussions", "guides", "pages", "comparisons", "events",
	"jobs", "stories", "achievements", "services", "learn", "docs",
	"spaces", "tags", "comments",
}

// folderPosition returns the position for a default folder name, or 0 if
// the name is not in the canonical order.
func folderPosition(name string) int32 {
	for i, n := range defaultFolderOrder {
		if n == name {
			return int32(i) //nolint:gosec // position is bounded by the small default folder list
		}
	}

	return 0
}

// Resolve returns the default FolderResponse for a bookmark of the given
// entity and (for content) content type. If no default mapping exists, it
// returns an empty FolderResponse with a zero ID. If the mapped folder does
// not yet exist for the user, it is created.
func (r *DefaultFolderResolver) Resolve(ctx context.Context, userID pgtype.UUID, entityType, contentType string) (FolderResponse, error) {
	name := r.resolveName(entityType, contentType)
	if name == "" {
		return FolderResponse{}, nil
	}

	return r.findOrCreateFolder(ctx, userID, name)
}

// resolveName returns the default folder name for the given entity/content
// type pair. Empty means the item should remain unfiled.
func (r *DefaultFolderResolver) resolveName(entityType, contentType string) string {
	switch entityType {
	case "content", "blog_post":
		if n, ok := contentTypeFolderNames[contentType]; ok {
			return n
		}

		if n, ok := entityTypeFolderNames[entityType]; ok {
			return n
		}

		return ""
	default:
		if n, ok := entityTypeFolderNames[entityType]; ok {
			return n
		}

		return ""
	}
}

// findOrCreateFolder returns the existing folder with the given name, or
// creates it with the canonical default position. The name is already
// validated as a known default folder.
func (r *DefaultFolderResolver) findOrCreateFolder(ctx context.Context, userID pgtype.UUID, name string) (FolderResponse, error) {
	folders, err := r.service.ListFolders(ctx, userID)
	if err != nil {
		return FolderResponse{}, fmt.Errorf("listing folders for default resolution: %w", err)
	}

	for _, f := range folders {
		if f.Name == name {
			return f, nil
		}
	}

	folder, err := r.service.CreateFolder(ctx, userID, name)
	if err == nil {
		return folderResponseFromModel(folder), nil
	}

	// The folder may have been created by a concurrent request between the
	// list and the insert. Re-list once and return the existing folder.
	if errors.Is(err, errpkg.ErrDuplicate) {
		folders, err = r.service.ListFolders(ctx, userID)
		if err != nil {
			return FolderResponse{}, fmt.Errorf("re-listing folders after duplicate: %w", err)
		}

		for _, f := range folders {
			if f.Name == name {
				return f, nil
			}
		}
	}

	return FolderResponse{}, fmt.Errorf("creating default folder %q: %w", name, err)
}

// CreateDefaultFoldersForUser pre-seeds all default folders for a user at
// registration. It is idempotent — existing folders are left untouched.
func (r *DefaultFolderResolver) CreateDefaultFoldersForUser(ctx context.Context, userID pgtype.UUID) error {
	for _, name := range defaultFolderOrder {
		if _, err := r.findOrCreateFolder(ctx, userID, name); err != nil {
			return fmt.Errorf("pre-seeding default folder %q: %w", name, err)
		}
	}

	return nil
}

// Resolver returns the service's default folder resolver. Callers (content,
// users, forum services) can type-assert to the minimal interface they need.
func (s *Service) Resolver() *DefaultFolderResolver {
	return NewDefaultFolderResolver(s)
}

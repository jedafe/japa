// Package bookmarkfolders — first-class named bookmark folders (catalog:
// GET/POST /me/bookmark-folders, PATCH/DELETE /me/bookmark-folders/:id).
//
// Distinct from bookmarks.folder (a free-text column added in migration
// 000103) — that denormalized string is untouched; reconciling the two is a
// separate decision from wiring this CRUD.
package bookmarkfolders

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Service provides bookmark folder CRUD.
type Service struct {
	read  *gen.Queries
	write *gen.Queries
}

// NewService creates a Service backed by the given database.
func NewService(database *db.DB) *Service {
	return &Service{read: gen.New(database.Reader), write: gen.New(database.Writer)}
}

// CreateFolder creates a new named bookmark folder for the user.
func (s *Service) CreateFolder(ctx context.Context, tenantID, userID pgtype.UUID, name string) (gen.BookmarkFolder, error) {
	folder, err := s.write.CreateBookmarkFolder(ctx, gen.CreateBookmarkFolderParams{
		TenantID: tenantID,
		UserID:   userID,
		Name:     name,
	})
	if err != nil {
		return gen.BookmarkFolder{}, fmt.Errorf("creating bookmark folder: %w", err)
	}

	return folder, nil
}

// FolderListItem is a bookmark folder with its bookmark count.
type FolderListItem struct {
	ID            pgtype.UUID        `json:"id"`
	TenantID      pgtype.UUID        `json:"tenant_id"`
	UserID        pgtype.UUID        `json:"user_id"`
	Name          string             `json:"name"`
	CreatedAt     pgtype.Timestamptz `json:"created_at"`
	UpdatedAt     pgtype.Timestamptz `json:"updated_at"`
	BookmarkCount int64              `json:"bookmark_count"`
}

// ListFolders returns every bookmark folder for the user, alphabetically,
// with a bookmark_count per folder (BL-011).
func (s *Service) ListFolders(ctx context.Context, tenantID, userID pgtype.UUID) ([]FolderListItem, error) {
	rows, err := s.read.ListBookmarkFoldersByUser(ctx, gen.ListBookmarkFoldersByUserParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing bookmark folders: %w", err)
	}

	folders := make([]FolderListItem, len(rows))
	for i, r := range rows {
		folders[i] = FolderListItem{
			ID:            r.ID,
			TenantID:      r.TenantID,
			UserID:        r.UserID,
			Name:          r.Name,
			CreatedAt:     r.CreatedAt,
			UpdatedAt:     r.UpdatedAt,
			BookmarkCount: r.BookmarkCount,
		}
	}

	return folders, nil
}

// GetFolder retrieves a single folder, validating ownership (BL-013).
// Returns ErrNotFound if the folder doesn't exist or belongs to another user.
func (s *Service) GetFolder(ctx context.Context, id, tenantID, userID pgtype.UUID) (gen.BookmarkFolder, error) {
	folder, err := s.read.GetBookmarkFolder(ctx, gen.GetBookmarkFolderParams{
		ID:       id,
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return gen.BookmarkFolder{}, fmt.Errorf("%w: %w", errpkg.ErrNotFound, err)
	}

	return folder, nil
}

// UpdateFolder renames a bookmark folder. Returns ErrNotFound if it doesn't
// exist or doesn't belong to the caller.
func (s *Service) UpdateFolder(ctx context.Context, id, tenantID, userID pgtype.UUID, name string) (gen.BookmarkFolder, error) {
	folder, err := s.write.UpdateBookmarkFolder(ctx, gen.UpdateBookmarkFolderParams{
		Name:     name,
		ID:       id,
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return gen.BookmarkFolder{}, fmt.Errorf("%w: %w", errpkg.ErrNotFound, err)
	}

	return folder, nil
}

// DeleteFolder removes a bookmark folder. Returns ErrNotFound if it doesn't
// exist or doesn't belong to the caller.
func (s *Service) DeleteFolder(ctx context.Context, id, tenantID, userID pgtype.UUID) error {
	affected, err := s.write.DeleteBookmarkFolder(ctx, gen.DeleteBookmarkFolderParams{
		ID:       id,
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return fmt.Errorf("deleting bookmark folder: %w", err)
	}

	if affected == 0 {
		return errpkg.ErrNotFound
	}

	return nil
}

// Package bookmarkfolders — first-class named bookmark folders (catalog:
// GET/POST /me/bookmark-folders, PATCH/DELETE /me/bookmark-folders/:id).
//
// Distinct from bookmarks.folder (a free-text column added in migration
// 000103) — that denormalized string is untouched; reconciling the two is a
// separate decision from wiring this CRUD.
package bookmarkfolders

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// MaxFolderNameLength is the upper bound on a bookmark folder name. §08 does
// not define a max length, so this sane default prevents unbounded text input
// from reaching the DB (BE-04).
const MaxFolderNameLength = 100

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
// Returns ErrDuplicate when a folder with the same name already exists for
// this user (UNIQUE(user_id, name) violation — BE-01).
func (s *Service) CreateFolder(ctx context.Context, tenantID, userID pgtype.UUID, name string) (gen.BookmarkFolder, error) {
	folder, err := s.write.CreateBookmarkFolder(ctx, gen.CreateBookmarkFolderParams{
		TenantID: tenantID,
		UserID:   userID,
		Name:     name,
		Position: 0,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return gen.BookmarkFolder{}, fmt.Errorf("%w: bookmark folder name already exists: %w", errpkg.ErrDuplicate, err)
		}

		return gen.BookmarkFolder{}, fmt.Errorf("creating bookmark folder: %w", err)
	}

	return folder, nil
}

// FolderResponse is the canonical API shape for a bookmark folder, returned
// by every /me/bookmark-folders endpoint (list, create, update). It omits
// tenant_id/user_id (the caller already knows their own identity — defense
// in depth) and includes position + bookmark_count so the shape is identical
// across List, Create, and Update (BF-04/BF-05).
type FolderResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Position      int32  `json:"position"`
	BookmarkCount int64  `json:"bookmark_count"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// folderResponseFromRow builds a FolderResponse from a ListBookmarkFoldersByUserRow.
func folderResponseFromRow(r gen.ListBookmarkFoldersByUserRow) FolderResponse {
	return FolderResponse{
		ID:            r.ID.String(),
		Name:          r.Name,
		Position:      r.Position,
		BookmarkCount: r.BookmarkCount,
		CreatedAt:     r.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:     r.UpdatedAt.Time.Format(time.RFC3339),
	}
}

// folderResponseFromModel builds a FolderResponse from a BookmarkFolder model
// (returned by Create/Update/Get). bookmark_count is 0 because the
// single-row queries do not join bookmarks; the list endpoint is the source
// of counts.
func folderResponseFromModel(f gen.BookmarkFolder) FolderResponse {
	return FolderResponse{
		ID:            f.ID.String(),
		Name:          f.Name,
		Position:      f.Position,
		BookmarkCount: 0,
		CreatedAt:     f.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:     f.UpdatedAt.Time.Format(time.RFC3339),
	}
}

// ListFolders returns every bookmark folder for the user, ordered by position
// then name, with a bookmark_count per folder (BL-011).
func (s *Service) ListFolders(ctx context.Context, tenantID, userID pgtype.UUID) ([]FolderResponse, error) {
	rows, err := s.read.ListBookmarkFoldersByUser(ctx, gen.ListBookmarkFoldersByUserParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing bookmark folders: %w", err)
	}

	folders := make([]FolderResponse, len(rows))
	for i, r := range rows {
		folders[i] = folderResponseFromRow(r)
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
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.BookmarkFolder{}, fmt.Errorf("%w: bookmark folder: %w", errpkg.ErrNotFound, err)
		}

		return gen.BookmarkFolder{}, fmt.Errorf("getting bookmark folder: %w", err)
	}

	return folder, nil
}

// UpdateFolder renames a bookmark folder. Returns ErrNotFound if it doesn't
// exist or doesn't belong to the caller; ErrDuplicate if the new name is
// already used by another folder owned by this user (BE-02).
func (s *Service) UpdateFolder(ctx context.Context, id, tenantID, userID pgtype.UUID, name string) (gen.BookmarkFolder, error) {
	folder, err := s.write.UpdateBookmarkFolder(ctx, gen.UpdateBookmarkFolderParams{
		Name:     name,
		ID:       id,
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.BookmarkFolder{}, fmt.Errorf("%w: bookmark folder: %w", errpkg.ErrNotFound, err)
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return gen.BookmarkFolder{}, fmt.Errorf("%w: bookmark folder name already exists: %w", errpkg.ErrDuplicate, err)
		}

		return gen.BookmarkFolder{}, fmt.Errorf("updating bookmark folder: %w", err)
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

// ReorderFolders sets the position of each folder by its index in the
// folderIDs slice (BUG-008: §08 spec supports manual folder ordering via
// the position column). All folderIDs must belong to the caller + tenant;
// an unknown ID is silently skipped (defense-in-depth — the UPDATE is
// tenant+owner scoped). Positions are 0-indexed.
func (s *Service) ReorderFolders(ctx context.Context, tenantID, userID pgtype.UUID, folderIDs []pgtype.UUID) error {
	for i, fid := range folderIDs {
		if err := s.write.UpdateBookmarkFolderPosition(ctx, gen.UpdateBookmarkFolderPositionParams{
			Position: int32(i), // #nosec
			ID:       fid,
			TenantID: tenantID,
			UserID:   userID,
		}); err != nil {
			return fmt.Errorf("reordering bookmark folders at index %d: %w", i, err)
		}
	}

	return nil
}

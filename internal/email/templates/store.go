package templates

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store is an exported type.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore is an exported function.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

func (s *Store) listTemplates(ctx context.Context, filterType, filterIsActive string, limit, offset int32) ([]gen.ListEmailTemplatesRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListEmailTemplates(ctx, gen.ListEmailTemplatesParams{
		Type: filterType, IsActive: filterIsActive, PageOffset: offset, PageLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing email templates: %w", err)
	}

	return rows, nil
}

func (s *Store) countTemplates(ctx context.Context, filterType, filterIsActive string) (int32, error) {
	count, err := s.read.CountEmailTemplates(ctx, gen.CountEmailTemplatesParams{
		Type: filterType, IsActive: filterIsActive,
	})
	if err != nil {
		return 0, fmt.Errorf("counting email templates: %w", err)
	}

	return count, nil
}

func (s *Store) getTemplate(ctx context.Context, id pgtype.UUID) (gen.GetEmailTemplateRow, error) {
	row, err := s.read.GetEmailTemplate(ctx, id)
	if err != nil {
		return gen.GetEmailTemplateRow{}, fmt.Errorf("getting email template: %w", err)
	}

	return row, nil
}

func (s *Store) getTemplateByType(ctx context.Context, templateType string) (gen.GetEmailTemplateByTypeRow, error) {
	row, err := s.read.GetEmailTemplateByType(ctx, templateType)
	if err != nil {
		return gen.GetEmailTemplateByTypeRow{}, fmt.Errorf("getting email template by type: %w", err)
	}

	return row, nil
}

func (s *Store) updateTemplate(ctx context.Context, p gen.UpdateEmailTemplateParams) (gen.UpdateEmailTemplateRow, error) {
	row, err := s.write.UpdateEmailTemplate(ctx, p)
	if err != nil {
		return gen.UpdateEmailTemplateRow{}, fmt.Errorf("updating email template: %w", err)
	}

	return row, nil
}

func (s *Store) resetTemplate(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.ResetEmailTemplate(ctx,
		id,
	); err != nil {
		return fmt.Errorf("resetting email template: %w", err)
	}

	return nil
}

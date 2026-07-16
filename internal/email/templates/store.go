package templates

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

func (s *Store) listTemplates(ctx context.Context, tenantID pgtype.UUID, filterType string, limit, offset int32) ([]gen.EmailTemplate, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}
	rows, err := s.read.ListEmailTemplates(ctx, gen.ListEmailTemplatesParams{
		TenantID: tenantID, Type: filterType, PageOffset: offset, PageLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing email templates: %w", err)
	}
	return rows, nil
}

func (s *Store) countTemplates(ctx context.Context, tenantID pgtype.UUID, filterType string) (int32, error) {
	count, err := s.read.CountEmailTemplates(ctx, gen.CountEmailTemplatesParams{
		TenantID: tenantID, Type: filterType,
	})
	if err != nil {
		return 0, fmt.Errorf("counting email templates: %w", err)
	}
	return count, nil
}

func (s *Store) getTemplate(ctx context.Context, id, tenantID pgtype.UUID) (gen.EmailTemplate, error) {
	row, err := s.read.GetEmailTemplate(ctx, gen.GetEmailTemplateParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.EmailTemplate{}, fmt.Errorf("getting email template: %w", err)
	}
	return row, nil
}

func (s *Store) getTemplateByType(ctx context.Context, tenantID pgtype.UUID, templateType string) (gen.EmailTemplate, error) {
	row, err := s.read.GetEmailTemplateByType(ctx, gen.GetEmailTemplateByTypeParams{
		TenantID: tenantID, Type: templateType,
	})
	if err != nil {
		return gen.EmailTemplate{}, fmt.Errorf("getting email template by type: %w", err)
	}
	return row, nil
}

func (s *Store) updateTemplate(ctx context.Context, p gen.UpdateEmailTemplateParams) (gen.EmailTemplate, error) {
	row, err := s.write.UpdateEmailTemplate(ctx, p)
	if err != nil {
		return gen.EmailTemplate{}, fmt.Errorf("updating email template: %w", err)
	}
	return row, nil
}

func (s *Store) resetTemplate(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.ResetEmailTemplate(ctx, gen.ResetEmailTemplateParams{
		ID: id, TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("resetting email template: %w", err)
	}
	return nil
}

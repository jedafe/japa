package templates

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

type Service struct {
	store *Store
	db    *db.DB
}

func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{read: gen.New(database.Reader), write: gen.New(database.Writer)},
		db:    database,
	}
}

type ListTemplatesResponse struct {
	Items      []gen.ListEmailTemplatesRow `json:"items"`
	Page       int32                       `json:"page"`
	Total      int64                       `json:"total"`
	TotalPages int32                       `json:"total_pages"`
}

func (s *Service) ListTemplates(ctx context.Context, tenantID pgtype.UUID, filterType string, limit, offset int32) (*ListTemplatesResponse, error) {
	templates, err := s.store.listTemplates(ctx, tenantID, filterType, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing templates: %w", err)
	}
	total, err := s.store.countTemplates(ctx, tenantID, filterType)
	if err != nil {
		return nil, fmt.Errorf("counting templates: %w", err)
	}
	totalPages := total / limit
	if total%limit > 0 {
		totalPages++
	}
	return &ListTemplatesResponse{Items: templates, Page: offset / limit, Total: int64(total), TotalPages: totalPages}, nil
}

func (s *Service) GetTemplate(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetEmailTemplateRow, error) {
	return s.store.getTemplate(ctx, id, tenantID)
}

func (s *Service) GetTemplateByType(ctx context.Context, tenantID pgtype.UUID, templateType string) (gen.GetEmailTemplateByTypeRow, error) {
	return s.store.getTemplateByType(ctx, tenantID, templateType)
}

func (s *Service) UpdateTemplate(ctx context.Context, id, tenantID pgtype.UUID, name, subject, htmlBody, plainTextBody, status string) (gen.UpdateEmailTemplateRow, error) {
	return s.store.updateTemplate(ctx, gen.UpdateEmailTemplateParams{
		ID:             id,
		TenantID:       tenantID,
		Name:           name,
		Subject:        subject,
		HtmlBody:       htmlBody,
		PlainTextBody:  plainTextBody,
		Status:         status,
	})
}

func (s *Service) ResetTemplate(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.resetTemplate(ctx, id, tenantID)
}

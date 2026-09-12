package templates

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Service is an exported type.
type Service struct {
	store *Store
	db    *db.DB
}

// NewService is an exported function.
func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{read: gen.New(database.Reader), write: gen.New(database.Writer)},
		db:    database,
	}
}

// ListTemplatesResponse is an exported type.
type ListTemplatesResponse struct {
	Items      []gen.ListEmailTemplatesRow `json:"items"`
	Page       int32                       `json:"page"`
	Total      int64                       `json:"total"`
	TotalPages int32                       `json:"total_pages"`
}

// ListTemplates is an exported method.
func (s *Service) ListTemplates(ctx context.Context, tenantID pgtype.UUID, filterType, filterIsActive string, limit, offset int32) (*ListTemplatesResponse, error) {
	templates, err := s.store.listTemplates(ctx, tenantID, filterType, filterIsActive, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing templates: %w", err)
	}

	total, err := s.store.countTemplates(ctx, tenantID, filterType, filterIsActive)
	if err != nil {
		return nil, fmt.Errorf("counting templates: %w", err)
	}

	totalPages := total / limit
	if total%limit > 0 {
		totalPages++
	}

	return &ListTemplatesResponse{Items: templates, Page: offset / limit, Total: int64(total), TotalPages: totalPages}, nil
}

// GetTemplate is an exported method.
func (s *Service) GetTemplate(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetEmailTemplateRow, error) {
	return s.store.getTemplate(ctx, id, tenantID)
}

// GetTemplateByType is an exported method.
func (s *Service) GetTemplateByType(ctx context.Context, tenantID pgtype.UUID, templateType string) (gen.GetEmailTemplateByTypeRow, error) {
	return s.store.getTemplateByType(ctx, tenantID, templateType)
}

// UpdateTemplate is an exported method.
func (s *Service) UpdateTemplate(ctx context.Context, id, tenantID pgtype.UUID, subject, htmlBody, plainTextBody string, isActive bool) (gen.UpdateEmailTemplateRow, error) {
	status := "inactive"
	if isActive {
		status = "active"
	}

	return s.store.updateTemplate(ctx, gen.UpdateEmailTemplateParams{
		ID: id,

		Subject:       subject,
		HtmlBody:      htmlBody,
		PlainTextBody: plainTextBody,
		Status:        status,
		IsActive:      isActive,
	})
}

// ResetTemplate is an exported method.
func (s *Service) ResetTemplate(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.resetTemplate(ctx, id, tenantID)
}

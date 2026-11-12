package templates

import (
	"context"
	"fmt"
	"html/template"
	texttemplate "text/template"

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
func (s *Service) ListTemplates(ctx context.Context, filterType, filterIsActive string, limit, offset int32) (*ListTemplatesResponse, error) {
	templates, err := s.store.listTemplates(ctx, filterType, filterIsActive, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing templates: %w", err)
	}

	total, err := s.store.countTemplates(ctx, filterType, filterIsActive)
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
func (s *Service) GetTemplate(ctx context.Context, id pgtype.UUID) (gen.GetEmailTemplateRow, error) {
	return s.store.getTemplate(ctx, id)
}

// UpdateTemplate is an exported method.
func (s *Service) UpdateTemplate(ctx context.Context, id pgtype.UUID, subject, htmlBody, plainTextBody string, isActive bool) (gen.UpdateEmailTemplateRow, error) {
	// ADR-055: validate Go template syntax at save time. An unparseable
	// template would otherwise persist and silently degrade to the
	// fallback body at send time.
	if _, err := texttemplate.New("subject").Parse(subject); err != nil {
		return gen.UpdateEmailTemplateRow{}, fmt.Errorf("%w: invalid subject template: %v", ErrInvalidTemplate, err)
	}

	if htmlBody != "" {
		if _, err := template.New("html_body").Parse(htmlBody); err != nil {
			return gen.UpdateEmailTemplateRow{}, fmt.Errorf("%w: invalid html_body template: %v", ErrInvalidTemplate, err)
		}
	}

	return s.store.updateTemplate(ctx, gen.UpdateEmailTemplateParams{
		ID: id,

		Subject:       subject,
		HtmlBody:      htmlBody,
		PlainTextBody: plainTextBody,
		IsActive:      isActive,
	})
}

// ResetTemplate is an exported method.
func (s *Service) ResetTemplate(ctx context.Context, id pgtype.UUID) error {
	// The reset query is a bare UPDATE — verify the row exists first so a
	// nonexistent id returns 404 instead of a silent 200 no-op.
	if _, err := s.store.getTemplate(ctx, id); err != nil {
		return err
	}

	return s.store.resetTemplate(ctx, id)
}

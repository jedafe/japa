// Package server — studio_review_adapters.go wires the Studio HITL review
// lifecycle (approval/rejection) to the concrete tool-table publishers.
//
// Ref: ADR-076 Amendment 1 (Studio–Intelligence integration, Approach B)
// Ref: japaconnect-engineering/sections/75-ai-content-studio.md
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/arrivalchecklist"
	"github.com/jedafe/japa/internal/attestationguide"
	"github.com/jedafe/japa/internal/calendar"
	"github.com/jedafe/japa/internal/communitylocator"
	"github.com/jedafe/japa/internal/credentialchecker"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/docs"
	"github.com/jedafe/japa/internal/housingguide"
	"github.com/jedafe/japa/internal/languagetesthub"
	"github.com/jedafe/japa/internal/studio"
	"github.com/jedafe/japa/internal/taxcalculator"
	"github.com/jedafe/japa/internal/visaroutes"
	"github.com/jedafe/japa/internal/wiki"
)

// hitlReviewReaderAdapter adapts the ai.Store to studio.HITLReviewReader.
type hitlReviewReaderAdapter struct {
	store *ai.Store
}

// GetHITLReview loads an ai_hitl_reviews row and converts it to the studio
// package's minimal HITLReview shape.
func (a *hitlReviewReaderAdapter) GetHITLReview(ctx context.Context, tenantID, reviewID pgtype.UUID) (studio.HITLReview, error) {
	rev, err := a.store.GetHITLReview(ctx, tenantID, reviewID)
	if err != nil {
		return studio.HITLReview{}, err
	}

	return studio.HITLReview{
		ID:          rev.ID,
		ContentID:   rev.ContentID,
		ContentType: rev.ContentType,
		TargetTable: rev.TargetTable,
		Source:      rev.Source,
		Status:      rev.Status,
	}, nil
}

// toolTablePublisher implements studio.ToolTablePublisher. It routes each
// target_table to the dedicated tool service's HITL-approve method.
type toolTablePublisher struct {
	db              *db.DB
	arrivalSvc      *arrivalchecklist.Service
	communityOrgSvc *communitylocator.Service
	attestationSvc  *attestationguide.Service
	languageTestSvc *languagetesthub.Service
	taxSvc          *taxcalculator.Service
	credentialSvc   *credentialchecker.Service
	housingSvc      *housingguide.Service
	visaRouteSvc    *visaroutes.Service
	wikiSvc         *wiki.Service
	calendarSvc     *calendar.Service
	docsSvc         *docs.Service
}

// Publish calls the appropriate tool service to mark the AI-generated draft
// as reviewed and published. Unknown target tables are logged and skipped.
//
//nolint:gocyclo // dispatch switch over all tool-table target tables.
func (p *toolTablePublisher) Publish(ctx context.Context, tenantID, reviewerID pgtype.UUID, targetTable string, targetID pgtype.UUID, notes string) error {
	if !targetID.Valid {
		return errors.New("tool-table publisher: target_id is invalid")
	}

	switch targetTable {
	case "arrival_checklist_guides":
		_, err := p.arrivalSvc.ApproveGuide(ctx, targetID, tenantID, reviewerID, arrivalchecklist.ApproveGuideRequest{Notes: strPtrOrNil(notes)})
		return err

	case "attestation_pathways":
		_, err := p.attestationSvc.Approve(ctx, targetID, tenantID, reviewerID, notes)
		return err

	case "community_organisations":
		_, err := p.communityOrgSvc.ApproveOrg(ctx, targetID, tenantID, reviewerID)
		return err

	case "credential_pathways":
		_, err := p.credentialSvc.ApprovePathway(ctx, targetID, tenantID, reviewerID, strPtrOrNil(notes))
		return err

	case "housing_guides":
		_, err := p.housingSvc.Approve(ctx, targetID, tenantID, reviewerID, housingguide.ApproveGuideRequest{Notes: strPtrOrNil(notes)})
		return err

	case "language_test_requirements":
		return p.languageTestSvc.ApproveRequirement(ctx, targetID, tenantID, reviewerID, notes)

	case "tax_obligation_guides":
		_, err := p.taxSvc.ApproveGuide(ctx, targetID, tenantID, reviewerID, taxcalculator.ApproveGuideRequest{Notes: strPtrOrNil(notes)})
		return err

	case "visa_routes":
		_, err := p.visaRouteSvc.Approve(ctx, targetID, reviewerID, visaroutes.ApproveInput{Notes: strPtrOrNil(notes)})
		return err

	case "calendar_entries":
		return p.calendarSvc.AdminUpdate(ctx, targetID, tenantID, reviewerID, "published", notes)

	case "doc_pages":
		if err := p.docsSvc.ApproveDocPage(ctx, tenantID, targetID, reviewerID); err != nil {
			return err
		}

		return p.docsSvc.PublishDocPage(ctx, tenantID, targetID, reviewerID)

	case "wiki_pages":
		return p.publishWikiPage(ctx, tenantID, reviewerID, targetID, notes)

	default:
		slog.Warn("tool-table publisher: unknown target_table, skipping publish",
			"target_table", targetTable, "target_id", targetID)
		return fmt.Errorf("unknown target_table %q", targetTable)
	}
}

// publishWikiPage finds the latest pending revision for the page and approves
// it. This is necessary because wiki HITL targets the page, but the approval
// API works on revisions (ADR-088, §80).
func (p *toolTablePublisher) publishWikiPage(ctx context.Context, tenantID, reviewerID, pageID pgtype.UUID, notes string) error {
	var revID pgtype.UUID

	err := p.db.Reader.QueryRow(ctx, `
		SELECT id FROM wiki_revisions
		WHERE wiki_page_id = $1 AND status = 'pending'
		ORDER BY created_at DESC LIMIT 1`, pageID, ).Scan(&revID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("wiki: no pending revision for page %s", pageID)
		}

		return fmt.Errorf("wiki: lookup pending revision: %w", err)
	}

	_, err = p.wikiSvc.ApproveRevision(ctx, revID, pageID, tenantID, reviewerID, notes)

	return err
}

// strPtrOrNil returns a pointer to the string if non-empty, otherwise nil.
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

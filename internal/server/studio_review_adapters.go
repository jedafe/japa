// Package server — studio_review_adapters.go wires the Studio HITL review
// lifecycle (approval/rejection) to the concrete tool-table publishers.
//
// Ref: ADR-076 Amendment 1 (Studio–Intelligence integration, Approach B)
// Ref: japaconnect-engineering/sections/75-ai-content-studio.md
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/arrivalchecklist"
	"github.com/jedafe/japa/internal/attestationguide"
	"github.com/jedafe/japa/internal/calendar"
	"github.com/jedafe/japa/internal/cdn"
	"github.com/jedafe/japa/internal/communitylocator"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/credentialchecker"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
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
func (a *hitlReviewReaderAdapter) GetHITLReview(ctx context.Context, reviewID pgtype.UUID) (studio.HITLReview, error) {
	rev, err := a.store.GetHITLReview(ctx, reviewID)
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

// toolPublicPaths maps each HITL tool target table to the public URL
// prefixes that render it (detail path for id-keyed pages — "{id}" is
// substituted — plus the section listing root). Used for edge purge after
// publish (ADR-134); these entities are not contents rows so they cannot
// go through purgeEdgeContent. Slug-keyed pages (community-locator,
// wiki) purge the section root only — the detail slug isn't in the row id.
var toolPublicPaths = map[string][]string{
	"arrival_checklist_guides":   {"/tools/arrival-checklist"},
	"attestation_pathways":       {"/tools/attestation-guide/{id}", "/tools/attestation-guide"},
	"community_organisations":    {"/tools/community-locator"},
	"credential_pathways":        {"/tools/credential-checker"},
	"housing_guides":             {"/tools/housing-guide/{id}", "/tools/housing-guide"},
	"language_test_requirements": {"/tools/language-test"},
	"tax_obligation_guides":      {"/tools/tax-calculator"},
	"visa_routes":                {"/tools/visa-routes/{id}", "/tools/visa-routes"},
	"calendar_entries":           {"/calendar/{id}", "/calendar"},
	"wiki_pages":                 {"/wiki"},
}

// purgeToolPages invalidates a tool entity's public pages at the edge —
// fire-and-forget like every other purge call site (Rule 8).
func purgeToolPages(ctx context.Context, targetTable string, targetID pgtype.UUID) {
	if !cdn.Enabled() {
		return
	}

	paths := toolPublicPaths[targetTable]
	if len(paths) == 0 {
		return
	}

	base := cdn.SiteURL()
	if base == "" {
		return
	}

	req := cdn.PurgeRequest{Tags: []string{"tool:" + targetTable + ":" + targetID.String()}}
	for _, p := range paths {
		req.URLs = append(req.URLs, base+strings.ReplaceAll(p, "{id}", targetID.String()))
	}

	cdn.PurgeAsync(ctx, req)
}

// Publish calls the appropriate tool service to mark the AI-generated draft
// as reviewed and published. Unknown target tables are logged and skipped.
//
//nolint:gocyclo // dispatch switch over all tool-table target tables.
func (p *toolTablePublisher) Publish(ctx context.Context, reviewerID pgtype.UUID, targetTable string, targetID pgtype.UUID, notes string) error {
	if !targetID.Valid {
		return errors.New("tool-table publisher: target_id is invalid")
	}

	defer purgeToolPages(ctx, targetTable, targetID)

	switch targetTable {
	case "arrival_checklist_guides":
		_, err := p.arrivalSvc.ApproveGuide(ctx, targetID, reviewerID, arrivalchecklist.ApproveGuideRequest{Notes: strPtrOrNil(notes)})
		return err

	case "attestation_pathways":
		_, err := p.attestationSvc.Approve(ctx, targetID, reviewerID, notes)
		return err

	case "community_organisations":
		_, err := p.communityOrgSvc.ApproveOrg(ctx, targetID, reviewerID)
		return err

	case "credential_pathways":
		_, err := p.credentialSvc.ApprovePathway(ctx, targetID, reviewerID, strPtrOrNil(notes))
		return err

	case "housing_guides":
		_, err := p.housingSvc.Approve(ctx, targetID, reviewerID, housingguide.ApproveGuideRequest{Notes: strPtrOrNil(notes)})
		return err

	case "language_test_requirements":
		return p.languageTestSvc.ApproveRequirement(ctx, targetID, reviewerID, notes)

	case "tax_obligation_guides":
		_, err := p.taxSvc.ApproveGuide(ctx, targetID, reviewerID, taxcalculator.ApproveGuideRequest{Notes: strPtrOrNil(notes)})
		return err

	case "visa_routes":
		_, err := p.visaRouteSvc.Approve(ctx, targetID, reviewerID, visaroutes.ApproveInput{Notes: strPtrOrNil(notes)})
		return err

	case "calendar_entries":
		return p.calendarSvc.AdminUpdate(ctx, targetID, reviewerID, "published", notes)

	case "doc_pages":
		if err := p.docsSvc.ApproveDocPage(ctx, targetID, reviewerID); err != nil {
			return err
		}

		return p.docsSvc.PublishDocPage(ctx, targetID, reviewerID)

	case "wiki_pages":
		return p.publishWikiPage(ctx, reviewerID, targetID, notes)

	default:
		slog.Warn("tool-table publisher: unknown target_table, skipping publish",
			"target_table", targetTable, "target_id", targetID)
		return fmt.Errorf("unknown target_table %q", targetTable)
	}
}

// publishWikiPage finds the latest pending revision for the page and approves
// it. This is necessary because wiki HITL targets the page, but the approval
// API works on revisions (ADR-088, §80).
func (p *toolTablePublisher) publishWikiPage(ctx context.Context, reviewerID, pageID pgtype.UUID, notes string) error {
	var revID pgtype.UUID

	err := p.db.Reader.QueryRow(ctx, `
		SELECT id FROM wiki_revisions
		WHERE wiki_page_id = $1 AND status = 'pending'
		ORDER BY created_at DESC LIMIT 1`, pageID).Scan(&revID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("wiki: no pending revision for page %s", pageID)
		}

		return fmt.Errorf("wiki: lookup pending revision: %w", err)
	}

	_, err = p.wikiSvc.ApproveRevision(ctx, revID, pageID, reviewerID, notes)

	return err
}

// regenContentStore is the subset of *content.Service the regen-draft
// merger needs — satisfied by *content.Service directly; declared as an
// interface so the merge path is unit-testable.
type regenContentStore interface {
	GetContent(ctx context.Context, id pgtype.UUID) (gen.GetContentByIDRow, error)
	UpdateContent(ctx context.Context, id, editedBy pgtype.UUID, input content.UpdateContentInput, bypassEditWindow bool) (gen.UpdateContentRow, error)
	DeleteContent(ctx context.Context, id pgtype.UUID) error
}

// regenDraftMergerAdapter implements studio.RegenDraftMerger. Approving a
// jobless (regen) studio review applies the draft's regenerated
// body/field_values onto the original contents row referenced by
// _ai_provenance.previous_content_id, preserving the original's slug,
// status, geo, and visibility — then soft-deletes the draft (§75
// merge-onto-original semantics; the alternative is two live copies of the
// same guide).
type regenDraftMergerAdapter struct {
	contentSvc regenContentStore
}

// MergeRegenDraft returns (intelRecordID, merged, err). merged=false means
// the draft carries no previous_content_id provenance — not a regen draft —
// and the caller should publish it directly.
func (a *regenDraftMergerAdapter) MergeRegenDraft(ctx context.Context, draftID, reviewerID pgtype.UUID) (pgtype.UUID, bool, error) {
	draft, err := a.contentSvc.GetContent(ctx, draftID)
	if err != nil {
		return pgtype.UUID{}, false, fmt.Errorf("regen merge: get draft: %w", err)
	}

	var prevID, intelID pgtype.UUID

	aiModel := ""

	if len(draft.FieldValues) > 0 {
		var fv map[string]any
		if err := json.Unmarshal(draft.FieldValues, &fv); err == nil {
			if prov, ok := fv["_ai_provenance"].(map[string]any); ok {
				if raw, ok := prov["previous_content_id"].(string); ok {
					_ = prevID.Scan(raw)
				}

				if raw, ok := prov["intel_record_id"].(string); ok {
					_ = intelID.Scan(raw)
				}

				aiModel, _ = prov["ai_model"].(string)
			}
		}
	}

	if !prevID.Valid {
		return pgtype.UUID{}, false, nil
	}

	original, err := a.contentSvc.GetContent(ctx, prevID)
	if err != nil {
		return pgtype.UUID{}, false, fmt.Errorf("regen merge: get original content %s: %w", prevID, err)
	}

	hash := content.ComputeContentHash(draft.Body)
	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	_, err = a.contentSvc.UpdateContent(ctx, prevID, reviewerID, content.UpdateContentInput{
		Title:         draft.Title,
		Slug:          original.Slug,
		Body:          draft.Body,
		BodyHtml:      "",
		FieldValues:   draft.FieldValues,
		ContentHash:   &hash,
		GeoSpaceID:    original.GeoSpaceID,
		SeoIndexable:  &original.SeoIndexable,
		Status:        original.Status,
		Visibility:    original.Visibility,
		GuideCategory: original.GuideCategory,
		AIGeneratedAt: now,
		AIModel:       &aiModel,
	}, true) // bypassEditWindow: reviewer-driven merge
	if err != nil {
		return pgtype.UUID{}, false, fmt.Errorf("regen merge: update original: %w", err)
	}

	if err := a.contentSvc.DeleteContent(ctx, draftID); err != nil {
		// Non-fatal: the merge already landed; the leftover draft stays
		// unpublished and can be cleaned up manually.
		slog.Warn("regen merge: soft-delete draft failed", "draft_id", draftID, "error", err)
	}

	return intelID, true, nil
}

// hitlDraftPublisher implements ai.DraftPublishHook — it takes the target of
// a non-studio ai_hitl_reviews row live when the review is approved (§103
// "Approve → publish": agent_draft_request drafts enter the queue
// unpublished, so approval must publish or the draft is orphaned).
// contents targets publish via UpdateContentStatus; tool-table targets
// route through the same publisher used by studio approvals.
type hitlDraftPublisher struct {
	contentPub   studio.ContentPublisher
	toolTablePub studio.ToolTablePublisher
}

// PublishHITLTarget publishes the review target. An empty target_table is
// treated as 'contents' (the column default).
func (p *hitlDraftPublisher) PublishHITLTarget(ctx context.Context, reviewerID pgtype.UUID, targetTable string, targetID pgtype.UUID) error {
	if !targetID.Valid {
		return errors.New("hitl publish: no target id")
	}

	if targetTable == "" || targetTable == "contents" {
		if p.contentPub == nil {
			return errors.New("hitl publish: content publisher not wired")
		}

		_, err := p.contentPub.UpdateContentStatus(ctx, targetID, "published")

		return err
	}

	if p.toolTablePub == nil {
		return fmt.Errorf("hitl publish: tool-table publisher not wired for %s", targetTable)
	}

	return p.toolTablePub.Publish(ctx, reviewerID, targetTable, targetID, "")
}

// strPtrOrNil returns a pointer to the string if non-empty, otherwise nil.
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

package server

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/crscalculator"
	"github.com/jedafe/japa/internal/ecaguide"
	"github.com/jedafe/japa/internal/pofcalculator"
	"github.com/jedafe/japa/internal/sopgenerator"
)

// toolReviewAdapter implements admin.ToolReviewHook — it fans a
// moderation-queue approve/reject out to the §§107–110 tool services.
// Each service method returns pgx.ErrNoRows when the ID isn't one of its
// rows, so the adapter walks the entity list in order.
//
// Status semantics are entity-specific and preserved by the services:
// CRS ruleset approve → 'active' + atomic supersede of the prior ruleset;
// guides/samples → 'published'; community updates → 'approved'.
type toolReviewAdapter struct {
	crs *crscalculator.Service
	pof *pofcalculator.Service
	eca *ecaguide.Service
	sop *sopgenerator.Service
}

func (a *toolReviewAdapter) HandleToolReview(ctx context.Context, id, reviewerID pgtype.UUID, action, reason string) (bool, string, error) {
	approve := action == "approve"
	reject := action == "reject" || action == "delete"

	switch {
	// crs_rulesets
	case approve:
		if err := a.crs.ApproveRuleset(ctx, id, reviewerID); err == nil {
			return true, "active", nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return false, "", err
		}
	case reject:
		if err := a.crs.RejectRuleset(ctx, id, reviewerID, reason); err == nil {
			return true, "rejected", nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return false, "", err
		}
	}

	// pof_requirements
	if approve {
		if err := a.pof.ApproveRequirement(ctx, id, reviewerID, reason); err == nil {
			return true, "published", nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return false, "", err
		}
	} else if reject {
		if err := a.pof.RejectRequirement(ctx, id, reviewerID, reason); err == nil {
			return true, "rejected", nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return false, "", err
		}
	}

	// eca_guides
	if approve {
		if err := a.eca.ApproveGuide(ctx, id, reviewerID, reason); err == nil {
			return true, "published", nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return false, "", err
		}
	} else if reject {
		if err := a.eca.RejectGuide(ctx, id, reviewerID, reason); err == nil {
			return true, "rejected", nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return false, "", err
		}
	}

	// eca_community_updates — shared approve/reject entry point
	if approve || reject {
		status := "rejected"
		if approve {
			status = "approved"
		}
		if err := a.eca.ModerateUpdate(ctx, id, reviewerID, status); err == nil {
			return true, status, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return false, "", err
		}
	}

	// sop_samples
	if approve {
		if err := a.sop.ApproveSample(ctx, id, reviewerID, reason); err == nil {
			return true, "published", nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return false, "", err
		}
	} else if reject {
		if err := a.sop.RejectSample(ctx, id, reviewerID, reason); err == nil {
			return true, "rejected", nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return false, "", err
		}
	}

	// sop_templates
	if approve {
		if err := a.sop.ApproveTemplate(ctx, id, reviewerID); err == nil {
			return true, "published", nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return false, "", err
		}
	} else if reject {
		if err := a.sop.RejectTemplate(ctx, id); err == nil {
			return true, "rejected", nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return false, "", err
		}
	}

	return false, "", nil
}
